// Copyright 2013 Federico Sogaro. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webdriver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
	"sync"
)

type ChromeSwitches map[string]interface{}

type ChromeDriver struct {
	WebDriverCore
	//The port that ChromeDriver listens on. Default: 9515
	Port int
	//The URL path prefix to use for all incoming WebDriver REST requests. Default: ""
	BaseUrl string
	//The number of threads to use for handling HTTP requests. Default: 4
	Threads int
	//The path to use for the ChromeDriver server log. Default: ./chromedriver.log
	LogPath string
	// Log file to dump chromedriver stdout/stderr. If "" send to terminal. Default: ""
	LogFile string
	// Start method fails if Chromedriver doesn't start in less than StartTimeout. Default 20s.
	StartTimeout time.Duration
	// specifies whether chrome driver should be started in headless mode or not
	Headless bool

	path    string
	cmd     *exec.Cmd
	logFile *os.File
}

type ChromeOptions struct {
	Args       []string `json:"args,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

//var initialized bool = false
//var freePortsQueue chan int
//var portsMutex sync.Mutex
const FIRST_PORT_NUMBER = 9500
//we want to start chrome one by one, for chrome not to crash
var chromeMutex sync.Mutex

//create a new service using chromedriver.
//function returns an error if not supported switches are passed. Actual content
//of valid-named switches is not validate and is passed as it is.
//switch silent is removed (output is needed to check if chromedriver started correctly)
//We want to support a number of chrome drivers running in parallel, so we create each time a new chrome driver
func NewChromeDriver(path string, maxNumberOfWebdrivers int, headless bool) ChromeDriver {
	fmt.Println("Requesting new session")
	/*portsMutex.Lock()
	if !initialized {
		initialized = true
		freePortsQueue = make(chan int, maxNumberOfWebdrivers)
		for portNumber:=FIRST_PORT_NUMBER; portNumber<FIRST_PORT_NUMBER+maxNumberOfWebdrivers; portNumber++ {
			freePortsQueue <- portNumber
		}
	}
	portsMutex.Unlock()*/
	chromeDriver := ChromeDriver{}
	chromeDriver.path = path
	//chromeDriver.Port = <- freePortsQueue
	chromeDriver.Port = FIRST_PORT_NUMBER
	fmt.Println("Recieved port")
	chromeDriver.BaseUrl = ""
	chromeDriver.LogPath = "chromedriver.log"
	chromeDriver.StartTimeout = 20 * time.Second
	chromeDriver.Headless = headless

	return chromeDriver
}

var switchesFormat = "-port=%d -url-base=%s -log-path=%s -http-threads=%d"

var cmdchan = make(chan error)

func (chromeDriver *ChromeDriver) Start() error {
	fmt.Println("Starting new chrome driver")
	csferr := "chromedriver start failed: "
	if chromeDriver.cmd != nil {
		return errors.New(csferr + "chromedriver already running")
	}

	if chromeDriver.LogPath != "" {
		//check if log-path is writable
		file, err := os.OpenFile(chromeDriver.LogPath, os.O_WRONLY|os.O_CREATE, 0664)
		if err != nil {
			return errors.New(csferr + "unable to write in log path: " + err.Error())
		}
		file.Close()
	}

	chromeDriver.url = fmt.Sprintf("http://127.0.0.1:%d%s", chromeDriver.Port, chromeDriver.BaseUrl)
	//this is an error in fedesog's implementation, these switches are not supported, this is not the way to pass them
	var switches []string
	switches = append(switches, "--port="+strconv.Itoa(chromeDriver.Port))
	switches = append(switches, "--log-path="+chromeDriver.LogPath)
	switches = append(switches, "--http-threads="+strconv.Itoa(chromeDriver.Threads))
	switches = append(switches, "--disable-dev-shm-usage"+strconv.Itoa(chromeDriver.Threads))

	chromeDriver.cmd = exec.Command(chromeDriver.path, switches...)
	//chromeDriver.cmd = exec.Command(chromeDriver.path)
	stdout, err := chromeDriver.cmd.StdoutPipe()
	if err != nil {
		return errors.New(csferr + err.Error())
	}
	stderr, err := chromeDriver.cmd.StderrPipe()
	if err != nil {
		return errors.New(csferr + err.Error())
	}
	if err := chromeDriver.cmd.Start(); err != nil {
		return errors.New(csferr + err.Error())
	}
	if chromeDriver.LogFile != "" {
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		chromeDriver.logFile, err = os.OpenFile(chromeDriver.LogFile, flags, 0640)
		if err != nil {
			return err
		}
		go io.Copy(chromeDriver.logFile, stdout)
		go io.Copy(chromeDriver.logFile, stderr)
	} else {
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
	}
	if err = probePort(chromeDriver.Port, chromeDriver.StartTimeout); err != nil {
		fmt.Println("Error occured probing port: " + fmt.Sprint(chromeDriver.Port))
		return err
	}
	return nil
}

func (chromeDriver *ChromeDriver) Stop() error {
	fmt.Println("Stopping chrome driver...")
	if chromeDriver.cmd == nil {
		return errors.New("stop failed: chromedriver not running")
	}
	defer func() {
		chromeDriver.cmd = nil
		fmt.Println("Releasing port..")
		//freePortsQueue <- chromeDriver.Port
	}()
	chromeDriver.cmd.Process.Kill()
	chromeDriver.cmd.Process.Wait()
	//chromeDriver.cmd.Process.Signal(os.Kill)
	fmt.Println("Kill command sent")
	
	if chromeDriver.logFile != nil {
		chromeDriver.logFile.Close()
	}
	
	return nil
}

func (chromeDriver *ChromeDriver) NewSession(desired, required Capabilities) (*Session, error) {
	chromeMutex.Lock()
	defer chromeMutex.Unlock()
	//if we will want to support extenstions in the future, or other options it should be handled herein
	if (chromeDriver.Headless) {
		var chromeOptions ChromeOptions
		chromeOptions.Args = []string{"--headless", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox"}
		desired["chromeOptions"] = chromeOptions
	}
	session, err := chromeDriver.newSession(desired, required)
	if err != nil {
		return nil, err
	}
	session.wd = chromeDriver
	return session, nil
}

func (chromeDriver *ChromeDriver) Sessions() ([]Session, error) {
	sessions, err := chromeDriver.sessions()
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		sessions[i].wd = chromeDriver
	}
	return sessions, nil
}
