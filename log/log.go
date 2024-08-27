package log

import (
	"github.com/apex/log"
)

func Debugf(msg string, v ...interface{}) {
	log.Log.Debugf(msg, v...)
}

func Infof(msg string, v ...interface{}) {
	log.Log.Infof(msg, v...)
}

func Warnf(msg string, v ...interface{}) {
	log.Log.Infof(msg, v...)
}

func Errorf(msg string, v ...interface{}) {
	log.Log.Infof(msg, v...)
}

func Fatalf(msg string, v ...interface{}) {
	log.Log.Infof(msg, v...)
}
