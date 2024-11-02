// This package is a small go "library" (read: exec wrapper) around the
// mkfs.erofs binary that provides some useful primitives.
package erofs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/log"
	vrty "machinerun.io/atomfs/pkg/verity"
)

type erofsFuseInfoStruct struct {
	Path           string
	Version        string
	SupportsNotify bool
}

var once sync.Once
var erofsFuseInfo = erofsFuseInfoStruct{"", "", false}

func MakeErofs(tempdir string, rootfs string, eps *common.ExcludePaths, verity vrty.VerityMetadata) (io.ReadCloser, string, string, error) {
	var excludesFile string
	var err error
	var toExclude string
	var rootHash string

	if eps != nil {
		toExclude, err = eps.String()
		if err != nil {
			return nil, "", rootHash, errors.Wrapf(err, "couldn't create exclude path list")
		}
	}

	if len(toExclude) != 0 {
		excludes, err := os.CreateTemp(tempdir, "stacker-erofs-exclude-")
		if err != nil {
			return nil, "", rootHash, err
		}
		defer os.Remove(excludes.Name())

		excludesFile = excludes.Name()
		_, err = excludes.WriteString(toExclude)
		excludes.Close()
		if err != nil {
			return nil, "", rootHash, err
		}
	}

	tmpErofs, err := os.CreateTemp(tempdir, "stacker-erofs-img-")
	if err != nil {
		return nil, "", rootHash, err
	}
	// the following achieves the effect of creating a temporary file name
	// without actually creating the file;the goal being to provide a temporary
	// filename to provide to `mkfs.XXX` tool so we have a predictable name to
	// consume after `mkfs.XXX` has completed its task.
	//
	// NB: there's a TOCTOU here; something else can predict and produce
	// output in the tempfile name we created after we delete it and before
	// `mkfs.XXX` runs.
	tmpErofs.Close()
	os.Remove(tmpErofs.Name())

	defer os.Remove(tmpErofs.Name())

	args := []string{tmpErofs.Name(), rootfs}
	compression := LZ4HCCompression
	zstdOk, parallelOk := mkerofsSupportsFeature()
	if zstdOk {
		args = append(args, "-z", "zstd")
		compression = ZstdCompression
	}
	if parallelOk {
		args = append(args, "--workers", fmt.Sprintf("%d", runtime.NumCPU()))
	}
	if len(toExclude) != 0 {
		args = append(args, "--exclude-path", excludesFile)
	}
	cmd := exec.Command("mkfs.erofs", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return nil, "", rootHash, errors.Wrap(err, "couldn't build erofs")
	}

	if verity {
		rootHash, err = vrty.AppendVerityData(tmpErofs.Name())
		if err != nil {
			return nil, "", rootHash, err
		}
	}

	blob, err := os.Open(tmpErofs.Name())
	if err != nil {
		return nil, "", rootHash, errors.WithStack(err)
	}

	return blob, GenerateErofsMediaType(compression, verity), rootHash, nil
}

func findErofsFuseInfo() {
	var erofsPath string
	if p := which("erofsfuse"); p != "" {
		erofsPath = p
	} else {
		erofsPath = which("erofsfuse")
	}
	if erofsPath == "" {
		return
	}
	version, supportsNotify := erofsfuseSupportsMountNotification(erofsPath)
	log.Infof("Found erofsfuse at %s (version=%s notify=%t)", erofsPath, version, supportsNotify)
	erofsFuseInfo = erofsFuseInfoStruct{erofsPath, version, supportsNotify}
}

// erofsfuseSupportsMountNotification - returns true if erofsfuse supports mount
// notification, false otherwise
// erofsfuse is the path to the erofsfuse binary
func erofsfuseSupportsMountNotification(erofsfuse string) (string, bool) {
	cmd := exec.Command(erofsfuse)

	// `erofsfuse` always returns an error...  so we ignore it.
	out, _ := cmd.CombinedOutput()

	firstLine := strings.Split(string(out[:]), "\n")[0]
	version := strings.Split(firstLine, " ")[1]

	return version, false
}

var erofsNotFound = errors.Errorf("erofsfuse program not found")

// erofsFuse - mount erofsFile to extractDir
// return a pointer to the erofsfuse cmd.
// The caller of the this is responsible for the process created.
func erofsFuse(erofsFile, extractDir string) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	once.Do(findErofsFuseInfo)
	if erofsFuseInfo.Path == "" {
		return cmd, erofsNotFound
	}

	notifyOpts := ""
	notifyPath := ""
	if erofsFuseInfo.SupportsNotify {
		sockdir, err := os.MkdirTemp("", "sock")
		if err != nil {
			return cmd, err
		}
		defer os.RemoveAll(sockdir)
		notifyPath = filepath.Join(sockdir, "notifypipe")
		if err := syscall.Mkfifo(notifyPath, 0640); err != nil {
			return cmd, err
		}
		notifyOpts = "notify_pipe=" + notifyPath
	}

	// given extractDir of path/to/some/dir[/], log to path/to/some/.dir-erofs.log
	extractDir = strings.TrimSuffix(extractDir, "/")

	var cmdOut io.Writer
	var err error

	logf := filepath.Join(path.Dir(extractDir), "."+filepath.Base(extractDir)+"-erofsfuse.log")
	if cmdOut, err = os.OpenFile(logf, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644); err != nil {
		log.Infof("Failed to open %s for write: %v", logf, err)
		return cmd, err
	}

	fiPre, err := os.Lstat(extractDir)
	if err != nil {
		return cmd, errors.Wrapf(err, "Failed stat'ing %q", extractDir)
	}
	if fiPre.Mode()&os.ModeSymlink != 0 {
		return cmd, errors.Errorf("Refusing to mount onto a symbolic linkd")
	}

	// It would be nice to only enable debug (or maybe to only log to file at all)
	// if 'stacker --debug', but we do not have access to that info here.
	// to debug erofsfuse, use "allow_other,debug"
	optionArgs := "debug"
	if notifyOpts != "" {
		optionArgs += "," + notifyOpts
	}
	cmd = exec.Command(erofsFuseInfo.Path, "-f", "-o", optionArgs, erofsFile, extractDir)
	cmd.Stdin = nil
	cmd.Stdout = cmdOut
	cmd.Stderr = cmdOut
	cmdOut.Write([]byte(fmt.Sprintf("# %s\n", strings.Join(cmd.Args, " "))))
	if err != nil {
		return cmd, errors.Wrapf(err, "Failed writing to %s", logf)
	}
	log.Debugf("Extracting %s -> %s with %s [%s]", erofsFile, extractDir, erofsFuseInfo.Path, logf)
	err = cmd.Start()
	if err != nil {
		return cmd, err
	}

	// now poll/wait for one of 3 things to happen
	// a. child process exits - if it did, then some error has occurred.
	// b. the directory Entry is different than it was before the call
	//    to erofsfuse.  We have to do this because we do not have another
	//    way to know when the mount has been populated.
	//    https://github.com/vasi/erofsfuse/issues/49
	// c. a timeout (timeLimit) was hit
	startTime := time.Now()
	timeLimit := 30 * time.Second
	alarmCh := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(alarmCh)
	}()
	if erofsFuseInfo.SupportsNotify {
		notifyCh := make(chan byte)
		log.Infof("%s supports notify pipe, watching %q", erofsFuseInfo.Path, notifyPath)
		go func() {
			f, err := os.Open(notifyPath)
			if err != nil {
				return
			}
			defer f.Close()
			b1 := make([]byte, 1)
			for {
				n1, err := f.Read(b1)
				if err != nil {
					return
				}
				if err == nil && n1 >= 1 {
					break
				}
			}
			notifyCh <- b1[0]
		}()

		select {
		case <-alarmCh:
			cmd.Process.Kill()
			return cmd, errors.Wrapf(err, "Gave up on erofsfuse mount of %s with %s after %s", erofsFile, erofsFuseInfo.Path, timeLimit)
		case ret := <-notifyCh:
			if ret == 's' {
				return cmd, nil
			} else {
				return cmd, errors.Errorf("erofsfuse returned an error, check %s", logf)
			}
		}
	}
	for count := 0; !common.FileChanged(fiPre, extractDir); count++ {
		if cmd.ProcessState != nil {
			// process exited, the Wait() call in the goroutine above
			// caused ProcessState to be populated.
			return cmd, errors.Errorf("erofsfuse mount of %s with %s exited unexpectedly with %d", erofsFile, erofsFuseInfo.Path, cmd.ProcessState.ExitCode())
		}
		if time.Since(startTime) > timeLimit {
			cmd.Process.Kill()
			return cmd, errors.Wrapf(err, "Gave up on erofsfuse mount of %s with %s after %s", erofsFile, erofsFuseInfo.Path, timeLimit)
		}
		if count%10 == 1 {
			log.Debugf("%s is not yet mounted...(%s)", extractDir, time.Since(startTime))
		}
		time.Sleep(time.Duration(50 * time.Millisecond))
	}

	return cmd, nil
}

type ExtractPolicy struct {
	Extractors  []ErofsExtractor
	Extractor   ErofsExtractor
	Excuses     map[string]error
	initialized bool
	mutex       sync.Mutex
}

var exPolInfo struct {
	once   sync.Once
	err    error
	policy *ExtractPolicy
}

type ErofsExtractor interface {
	Name() string
	IsAvailable() error
	// Mount - Mount or extract path to dest.
	//   Return nil on "already extracted"
	//   Return error on failure.
	Mount(path, dest string) error
}

func NewExtractPolicy(args ...string) (*ExtractPolicy, error) {
	p := &ExtractPolicy{
		Extractors: []ErofsExtractor{},
		Excuses:    map[string]error{},
	}

	allEx := []ErofsExtractor{
		&KernelExtractor{},
		&ErofsFuseExtractor{},
		&FsckErofsExtractor{},
	}
	byName := map[string]ErofsExtractor{}
	for _, i := range allEx {
		byName[i.Name()] = i
	}

	for _, i := range args {
		extractor, ok := byName[i]
		if !ok {
			return nil, errors.Errorf("Unknown extractor: '%s'", i)
		}
		excuse := extractor.IsAvailable()
		if excuse != nil {
			p.Excuses[i] = excuse
			continue
		}
		p.Extractors = append(p.Extractors, extractor)
	}
	return p, nil
}

type FsckErofsExtractor struct {
	mutex sync.Mutex
}

func (k *FsckErofsExtractor) Name() string {
	return "fsck.erofs"
}

func (k *FsckErofsExtractor) IsAvailable() error {
	if which("fsck.erofs") == "" {
		return errors.Errorf("no 'fsck.erofs' in PATH")
	}
	return nil
}

func (k *FsckErofsExtractor) Mount(squashFile, extractDir string) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	// check if already extracted
	empty, err := isEmptyDir(extractDir)
	if err != nil {
		return errors.Wrapf(err, "Error checking for empty dir")
	}
	if !empty {
		return nil
	}

	log.Debugf("fsck.erofs %s -> %s", squashFile, extractDir)
	cmd := exec.Command("fsck.erofs", "-d", "--extract", extractDir, squashFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	err = cmd.Run()

	// on failure, remove the directory
	if err != nil {
		if rmErr := os.RemoveAll(extractDir); rmErr != nil {
			log.Errorf("Failed to remove %s after failed extraction of %s: %v", extractDir, squashFile, rmErr)
		}
		return err
	}

	// assert that extraction must create files. This way we can assume non-empty dir above
	// was populated by fsck.erofs.
	empty, err = isEmptyDir(extractDir)
	if err != nil {
		return errors.Errorf("Failed to read %s after successful extraction of %s: %v",
			extractDir, squashFile, err)
	}
	if empty {
		return errors.Errorf("%s was an empty fs image", squashFile)
	}

	return nil
}

type KernelExtractor struct {
	mutex sync.Mutex
}

func (k *KernelExtractor) Name() string {
	return "kmount"
}

func (k *KernelExtractor) IsAvailable() error {
	if !common.AmHostRoot() {
		return errors.Errorf("not host root")
	}
	return nil
}

func (k *KernelExtractor) Mount(squashFile, extractDir string) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if mounted, err := common.IsMountedAtDir(squashFile, extractDir); err != nil {
		return err
	} else if mounted {
		return nil
	}

	ecmd := []string{"mount", "-terofs", "-oloop,ro", squashFile, extractDir}
	var output bytes.Buffer
	cmd := exec.Command(ecmd[0], ecmd[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = &output
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	if err == nil {
		return nil
	}

	var retErr error

	exitError, ok := err.(*exec.ExitError)
	if !ok {
		retErr = errors.Errorf("kmount(%s) had unexpected error (no-rc), in exec (%v): %v",
			squashFile, ecmd, err)
	} else if status, ok := exitError.Sys().(syscall.WaitStatus); !ok {
		retErr = errors.Errorf("kmount(%s) had unexpected error (no-status), in exec (%v): %v",
			squashFile, ecmd, err)
	} else {
		retErr = errors.Errorf("kmount(%s) exited %d: %v", squashFile, status.ExitStatus(), output.String())
	}

	return retErr
}

type ErofsFuseExtractor struct {
	mutex sync.Mutex
}

func (k *ErofsFuseExtractor) Name() string {
	return "erofsfuse"
}

func (k *ErofsFuseExtractor) IsAvailable() error {
	once.Do(findErofsFuseInfo)
	if erofsFuseInfo.Path == "" {
		return errors.Errorf("no 'erofsfuse' in PATH")
	}
	return nil
}

func (k *ErofsFuseExtractor) Mount(erofsFile, extractDir string) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if mounted, err := common.IsMountedAtDir(erofsFile, extractDir); mounted && err == nil {
		log.Debugf("[%s] %s already mounted -> %s", k.Name(), erofsFile, extractDir)
		return nil
	} else if err != nil {
		return err
	}

	cmd, err := erofsFuse(erofsFile, extractDir)
	if err != nil {
		return err
	}

	log.Debugf("erofsfuse mounted (%d) %s -> %s", cmd.Process.Pid, erofsFile, extractDir)
	if err := cmd.Process.Release(); err != nil {
		return errors.Errorf("Failed to release process %s: %v", cmd, err)
	}
	return nil
}

// ExtractSingleErofsPolicy - extract squashfile to extractDir
func ExtractSingleErofsPolicy(squashFile, extractDir string, policy *ExtractPolicy) error {
	const initName = "init"
	if policy == nil {
		return errors.Errorf("policy cannot be nil")
	}

	// avoid taking a lock if already initialized (possibly premature optimization)
	if !policy.initialized {
		policy.mutex.Lock()
		// We may have been waiting on the initializer. If so, then the policy will now be initialized.
		// if not, then we are the initializer.
		if !policy.initialized {
			defer policy.mutex.Unlock()
			defer func() {
				policy.initialized = true
			}()
		} else {
			policy.mutex.Unlock()
		}
	}

	err := os.MkdirAll(extractDir, 0755)
	if err != nil {
		return err
	}

	fdest, err := filepath.Abs(extractDir)
	if err != nil {
		return err
	}

	if policy.initialized {
		if err, ok := policy.Excuses[initName]; ok {
			return err
		}
		return policy.Extractor.Mount(squashFile, fdest)
	}

	// At this point we are the initialzer
	if policy.Excuses == nil {
		policy.Excuses = map[string]error{}
	}

	if len(policy.Extractors) == 0 {
		policy.Excuses[initName] = errors.Errorf("policy had no extractors")
		return policy.Excuses[initName]
	}

	var extractor ErofsExtractor
	allExcuses := []string{}
	for _, extractor = range policy.Extractors {
		err = extractor.Mount(squashFile, fdest)
		if err == nil {
			policy.Extractor = extractor
			log.Debugf("Selected erofs extractor %s", extractor.Name())
			return nil
		}
		policy.Excuses[extractor.Name()] = err
	}

	for n, exc := range policy.Excuses {
		allExcuses = append(allExcuses, fmt.Sprintf("%s: %v", n, exc))
	}

	// nothing worked. populate Excuses[initName]
	policy.Excuses[initName] = errors.Errorf("No suitable extractor found:\n %s", strings.Join(allExcuses, "\n  "))
	return policy.Excuses[initName]
}

// ExtractSingleErofs - extract the squashFile to extractDir
// Initialize a extractPolicy struct and then call ExtractSingleErofsPolicy
// wik()th that.
func ExtractSingleErofs(squashFile string, extractDir string) error {
	exPolInfo.once.Do(func() {
		const envName = "STACKER_EROFS_EXTRACT_POLICY"
		const defPolicy = "kmount erofsfuse fsc.erofs"
		val := os.Getenv(envName)
		if val == "" {
			val = defPolicy
		}
		exPolInfo.policy, exPolInfo.err = NewExtractPolicy(strings.Fields(val)...)
		if exPolInfo.err == nil {
			for k, v := range exPolInfo.policy.Excuses {
				log.Debugf(" erofs extractor %s is not available: %v", k, v)
			}
		}
	})

	if exPolInfo.err != nil {
		return exPolInfo.err
	}

	return ExtractSingleErofsPolicy(squashFile, extractDir, exPolInfo.policy)
}

var checkSupported sync.Once
var zstdIsSuspported bool
var parallelIsSupported bool

func mkerofsSupportsFeature() (bool, bool) {
	checkSupported.Do(func() {
		var stdoutBuffer strings.Builder
		var stderrBuffer strings.Builder

		cmd := exec.Command("mkfs.erofs", "--help")
		cmd.Stdout = &stdoutBuffer
		cmd.Stderr = &stderrBuffer

		// Ignore errs here as `mkerofs --help` exit status code is 1
		_ = cmd.Run()

		if strings.Contains(stdoutBuffer.String(), "zstd") ||
			strings.Contains(stderrBuffer.String(), "zstd") {
			zstdIsSuspported = true
		}

		if strings.Contains(stdoutBuffer.String(), "workers") ||
			strings.Contains(stderrBuffer.String(), "workers") {
			parallelIsSupported = true
		}
	})

	return zstdIsSuspported, parallelIsSupported
}

func isEmptyDir(path string) (bool, error) {
	fh, err := os.Open(path)
	if err != nil {
		return false, err
	}

	_, err = fh.ReadDir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

// which - like the unix utility, return empty string for not-found.
// this might fit well in lib/, but currently lib's test imports
// erofs creating a import loop.
func which(name string) string {
	return whichSearch(name, strings.Split(os.Getenv("PATH"), ":"))
}

func whichSearch(name string, paths []string) string {
	var search []string

	if strings.ContainsRune(name, os.PathSeparator) {
		if filepath.IsAbs(name) {
			search = []string{name}
		} else {
			search = []string{"./" + name}
		}
	} else {
		search = []string{}
		for _, p := range paths {
			search = append(search, filepath.Join(p, name))
		}
	}

	for _, fPath := range search {
		if err := unix.Access(fPath, unix.X_OK); err == nil {
			return fPath
		}
	}

	return ""
}
