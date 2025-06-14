// This package is a small go "library" (read: exec wrapper) around the
// mksquashfs binary that provides some useful primitives.
package squashfs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/log"
	types "machinerun.io/atomfs/pkg/types"
	vrty "machinerun.io/atomfs/pkg/verity"
)

type squashFuseInfoStruct struct {
	Path           string
	Version        string
	SupportsNotify bool
}

var once sync.Once
var squashFuseInfo = squashFuseInfoStruct{"", "", false}

const PolicyEnvName = "STACKER_SQUASHFS_EXTRACT_POLICY"
const DefPolicies = "kmount squashfuse unsquashfs"
const AllPolicies = "kmount squashfuse unsquashfs"

func MakeSquashfs(tempdir string, rootfs string, eps *common.ExcludePaths, verity vrty.VerityMetadata) (io.ReadCloser, string, string, error) {
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
		excludes, err := os.CreateTemp(tempdir, "stacker-squashfs-exclude-")
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

	tmpSquashfs, err := os.CreateTemp(tempdir, "stacker-squashfs-img-")
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
	tmpSquashfs.Close()
	os.Remove(tmpSquashfs.Name())

	defer os.Remove(tmpSquashfs.Name())

	args := []string{rootfs, tmpSquashfs.Name()}
	compression := GzipCompression
	if mksquashfsSupportsZstd() {
		args = append(args, "-comp", "zstd")
		compression = ZstdCompression
	}
	if len(toExclude) != 0 {
		args = append(args, "-ef", excludesFile)
	}
	cmd := exec.Command("mksquashfs", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return nil, "", rootHash, errors.Wrap(err, "couldn't build squashfs")
	}

	if verity {
		rootHash, err = vrty.AppendVerityData(tmpSquashfs.Name())
		if err != nil {
			return nil, "", rootHash, err
		}
	}

	blob, err := os.Open(tmpSquashfs.Name())
	if err != nil {
		return nil, "", rootHash, errors.WithStack(err)
	}

	return blob, GenerateSquashfsMediaType(compression), rootHash, nil
}

func findSquashFuseInfo() {
	var sqfsPath string
	if p := common.Which("squashfuse_ll"); p != "" {
		sqfsPath = p
	} else {
		sqfsPath = common.Which("squashfuse")
	}
	if sqfsPath == "" {
		return
	}
	version, supportsNotify := sqfuseSupportsMountNotification(sqfsPath)
	log.Infof("Found squashfuse at %s (version=%s notify=%t)", sqfsPath, version, supportsNotify)
	squashFuseInfo = squashFuseInfoStruct{sqfsPath, version, supportsNotify}
}

// sqfuseSupportsMountNotification - returns true if squashfuse supports mount
// notification, false otherwise
// sqfuse is the path to the squashfuse binary
func sqfuseSupportsMountNotification(sqfuse string) (string, bool) {
	cmd := exec.Command(sqfuse)

	// `squashfuse` always returns an error...  so we ignore it.
	out, _ := cmd.CombinedOutput()

	firstLine := strings.Split(string(out[:]), "\n")[0]
	version := strings.Split(firstLine, " ")[1]
	v, err := semver.NewVersion(version)
	if err != nil {
		return version, false
	}
	// squashfuse notify mechanism was merged in 0.5.0
	constraint, err := semver.NewConstraint(">= 0.5.0")
	if err != nil {
		return version, false
	}
	if constraint.Check(v) {
		return version, true
	}
	return version, false
}

var squashFuseNotFound = errors.Errorf("squashfuse program not found")

// squashFuse - mount squashFile to extractDir
// return a pointer to the squashfuse cmd.
// The caller of the this is responsible for the process created.
func squashFuse(squashFile, extractDir string) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	once.Do(findSquashFuseInfo)
	if squashFuseInfo.Path == "" {
		return cmd, squashFuseNotFound
	}

	notifyOpts := ""
	notifyPath := ""
	if squashFuseInfo.SupportsNotify {
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

	// given extractDir of path/to/some/dir[/], log to path/to/some/.dir-squashfs.log
	extractDir = strings.TrimSuffix(extractDir, "/")

	var cmdOut io.Writer
	var err error

	logf := filepath.Join(path.Dir(extractDir), "."+filepath.Base(extractDir)+"-squashfuse.log")
	if cmdOut, err = os.OpenFile(logf, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644); err != nil {
		log.Errorf("Failed to open %s for write: %v", logf, err)
		return cmd, err
	}

	fiPre, err := os.Lstat(extractDir)
	if err != nil {
		return cmd, errors.Wrapf(err, "Failed stat'ing %q", extractDir)
	}
	if fiPre.Mode()&os.ModeSymlink != 0 {
		return cmd, errors.Errorf("Refusing to mount onto a symbolic link %q", extractDir)
	}

	// It would be nice to only enable debug (or maybe to only log to file at all)
	// if 'stacker --debug', but we do not have access to that info here.
	// to debug squashfuse, use "allow_other,debug"
	optionArgs := "allow_other,debug"
	if notifyOpts != "" {
		optionArgs += "," + notifyOpts
	}
	cmd = exec.Command(squashFuseInfo.Path, "-f", "-o", optionArgs, squashFile, extractDir)
	cmd.Stdin = nil
	cmd.Stdout = cmdOut
	cmd.Stderr = cmdOut
	_, err = cmdOut.Write([]byte(fmt.Sprintf("# %s\n", strings.Join(cmd.Args, " "))))
	if err != nil {
		return cmd, errors.Wrapf(err, "Failed writing to %s", logf)
	}
	log.Debugf("Extracting %s -> %s with %s [%s]", squashFile, extractDir, squashFuseInfo.Path, logf)
	err = cmd.Start()
	if err != nil {
		return cmd, err
	}

	// now poll/wait for one of 3 things to happen
	// a. child process exits - if it did, then some error has occurred.
	// b. the directory Entry is different than it was before the call
	//    to sqfuse.  We have to do this because we do not have another
	//    way to know when the mount has been populated.
	//    https://github.com/vasi/squashfuse/issues/49
	// c. a timeout (timeLimit) was hit
	startTime := time.Now()
	timeLimit := 30 * time.Second
	alarmCh := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(alarmCh)
	}()
	if squashFuseInfo.SupportsNotify {
		notifyCh := make(chan byte)
		log.Infof("%s supports notify pipe, watching %q", squashFuseInfo.Path, notifyPath)
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
				if n1 >= 1 {
					break
				}
			}
			notifyCh <- b1[0]
		}()

		select {
		case <-alarmCh:
			err = cmd.Process.Kill()
			return cmd, errors.Wrapf(err, "Gave up on squashFuse mount of %s with %s after %s", squashFile, squashFuseInfo.Path, timeLimit)
		case ret := <-notifyCh:
			if ret == 's' {
				return cmd, nil
			} else {
				return cmd, errors.Errorf("squashfuse returned an error, check %s", logf)
			}
		}
	}
	for count := 0; !common.FileChanged(fiPre, extractDir); count++ {
		if cmd.ProcessState != nil {
			// process exited, the Wait() call in the goroutine above
			// caused ProcessState to be populated.
			return cmd, errors.Errorf("squashFuse mount of %s with %s exited unexpectedly with %d", squashFile, squashFuseInfo.Path, cmd.ProcessState.ExitCode())
		}
		if time.Since(startTime) > timeLimit {
			err = cmd.Process.Kill()
			return cmd, errors.Wrapf(err, "Gave up on squashFuse mount of %s with %s after %s", squashFile, squashFuseInfo.Path, timeLimit)
		}
		if count%10 == 1 {
			log.Debugf("%s is not yet mounted...(%s)", extractDir, time.Since(startTime))
		}
		time.Sleep(time.Duration(50 * time.Millisecond))
	}

	return cmd, nil
}

type ExtractPolicy struct {
	Extractors  []types.FsExtractor
	Extractor   types.FsExtractor
	Excuses     map[string]error
	initialized bool
	mutex       sync.Mutex
}

var exPolInfo struct {
	once   sync.Once
	err    error
	policy *ExtractPolicy
}

func NewExtractPolicy(args ...string) (*ExtractPolicy, error) {
	p := &ExtractPolicy{
		Extractors: []types.FsExtractor{},
		Excuses:    map[string]error{},
	}

	allEx := []types.FsExtractor{
		&KernelExtractor{},
		&SquashFuseExtractor{},
		&UnsquashfsExtractor{},
	}
	byName := map[string]types.FsExtractor{}
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

type UnsquashfsExtractor struct {
	mutex sync.Mutex
}

func (k *UnsquashfsExtractor) Name() string {
	return "unsquashfs"
}

func (k *UnsquashfsExtractor) IsAvailable() error {
	if common.Which("unsquashfs") == "" {
		return errors.Errorf("no 'unsquashfs' in PATH")
	}
	return nil
}

func (k *UnsquashfsExtractor) Mount(squashFile, extractDir string) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	// check if already extracted
	empty, err := common.IsEmptyDir(extractDir)
	if err != nil {
		return errors.Wrapf(err, "Error checking for empty dir")
	}
	if !empty {
		return nil
	}

	log.Debugf("unsquashfs %s -> %s", squashFile, extractDir)
	cmd := exec.Command("unsquashfs", "-f", "-d", extractDir, squashFile)
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
	// was populated by unsquashfs.
	empty, err = common.IsEmptyDir(extractDir)
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

	ecmd := []string{"mount", "-tsquashfs", "-oloop,ro", squashFile, extractDir}
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

type SquashFuseExtractor struct {
	mutex sync.Mutex
}

func (k *SquashFuseExtractor) Name() string {
	return "squashfuse"
}

func (k *SquashFuseExtractor) IsAvailable() error {
	once.Do(findSquashFuseInfo)
	if squashFuseInfo.Path == "" {
		return errors.Errorf("no 'squashfuse' in PATH")
	}
	return nil
}

func (k *SquashFuseExtractor) Mount(squashFile, extractDir string) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if mounted, err := common.IsMountedAtDir(squashFile, extractDir); mounted && err == nil {
		log.Debugf("[%s] %s already mounted -> %s", k.Name(), squashFile, extractDir)
		return nil
	} else if err != nil {
		return err
	}

	cmd, err := squashFuse(squashFile, extractDir)
	if err != nil {
		return err
	}

	log.Debugf("squashFuse mounted (%d) %s -> %s", cmd.Process.Pid, squashFile, extractDir)
	if err := cmd.Process.Release(); err != nil {
		return errors.Errorf("Failed to release process %s: %v", cmd, err)
	}
	return nil
}

// ExtractSingleSquashPolicy - extract squashfile to extractDir
func ExtractSingleSquashPolicy(squashFile, extractDir string, policy *ExtractPolicy) error {
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

	var extractor types.FsExtractor
	allExcuses := []string{}
	for _, extractor = range policy.Extractors {
		err = extractor.Mount(squashFile, fdest)
		if err == nil {
			policy.Extractor = extractor
			log.Debugf("Selected squashfs extractor %s", extractor.Name())
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

// ExtractSingleSquash - extract the squashFile to extractDir
// Initialize a extractPolicy struct and then call ExtractSingleSquashPolicy
// wik()th that.
func ExtractSingleSquash(squashFile string, extractDir string) error {
	exPolInfo.once.Do(func() {
		val := os.Getenv(PolicyEnvName)
		if val == "" {
			val = DefPolicies
		}
		exPolInfo.policy, exPolInfo.err = NewExtractPolicy(strings.Fields(val)...)
		if exPolInfo.err == nil {
			for k, v := range exPolInfo.policy.Excuses {
				log.Debugf(" squashfs extractor %s is not available: %v", k, v)
			}
		}
	})

	if exPolInfo.err != nil {
		return exPolInfo.err
	}

	return ExtractSingleSquashPolicy(squashFile, extractDir, exPolInfo.policy)
}

var checkZstdSupported sync.Once
var zstdIsSuspported bool

func mksquashfsSupportsZstd() bool {
	checkZstdSupported.Do(func() {
		var stdoutBuffer strings.Builder
		var stderrBuffer strings.Builder

		cmd := exec.Command("mksquashfs", "--help")
		cmd.Stdout = &stdoutBuffer
		cmd.Stderr = &stderrBuffer

		// Ignore errs here as `mksquashfs --help` exit status code is 1
		_ = cmd.Run()

		if strings.Contains(stdoutBuffer.String(), "zstd") ||
			strings.Contains(stderrBuffer.String(), "zstd") {
			zstdIsSuspported = true
		}
	})

	return zstdIsSuspported
}
