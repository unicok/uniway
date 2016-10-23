package main

import (
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/davecgh/go-spew/spew"
)

func PrintPanicStack(extras ...interface{}) {
	if x := recover(); x != nil {
		log.Error(x)
		i := 0
		funcName, file, line, ok := runtime.Caller(i)
		for ok {
			log.WithFields(log.Fields{
				"frame": i,
				"func":  runtime.FuncForPC(funcName).Name(),
				"file":  file,
				"line":  line,
			}).Error("Panic stack")
			i++
			funcName, file, line, ok = runtime.Caller(i)
		}

		for k := range extras {
			log.WithFields(log.Fields{
				"exras": k,
				"data":  spew.Sdump(extras[k]),
			}).Error("extras dump")
		}
	}
}

// var (
// 	wg  sync.WaitGroup
// 	die = make(chan struct{})
// )

func signalWait(name string) {
	defer PrintPanicStack()

	if pid := syscall.Getpid(); pid != 1 {
		ioutil.WriteFile(name+".pid", []byte(strconv.Itoa(pid)), 0777)
		defer os.Remove(name + ".pid")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	for sig := range sigChan {
		switch sig {
		case syscall.SIGTERM, syscall.SIGINT:
			log.Println(name, "killed")
			return
		}
	}
}
