// SPDX-License-Identifier: Apache-2.0
/**
 * Copyright (c) 2024  Panasonic Automotive Systems, Co., Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"ucl-tools/internal/ucl"
	. "ucl-tools/internal/ulog"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func waitFileDetection(wg *sync.WaitGroup, retChan chan int, path string, pid int) {
	defer wg.Done()
	const countMax = 10
	for count := 0; count < countMax; count++ {
		process := ucl.CheckProcessAlive(pid)
		if process == nil {
			ELog.Println("Cannot find process: ", pid)
			retChan <- -1
			return
		} else {
			if fileExists(path) {
				retChan <- 0
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	ELog.Println("Timeout, Cannot find ", path)
	retChan <- -1
}

func waitRvgpuDevices(cardN int, pid int) int {
	var wg sync.WaitGroup
	deviceNum := 5
	retChan := make(chan int, deviceNum)
	wg.Add(deviceNum)
	go waitFileDetection(&wg, retChan, fmt.Sprintf("/dev/dri/rvgpu_virtio%d", cardN), pid)
	go waitFileDetection(&wg, retChan, fmt.Sprintf("/dev/input/rvgpu_touch%d", cardN), pid)
	go waitFileDetection(&wg, retChan, fmt.Sprintf("/dev/input/rvgpu_mouse%d", cardN), pid)
	go waitFileDetection(&wg, retChan, fmt.Sprintf("/dev/input/rvgpu_mouse_abs%d", cardN), pid)
	go waitFileDetection(&wg, retChan, fmt.Sprintf("/dev/input/rvgpu_keyboard%d", cardN), pid)
	wg.Wait()
	close(retChan)

	for result := range retChan {
		if result == -1 {
			return -1
		}
	}
	return 0
}

func isConnectionEstablished(pid int, ip string, port string, numSockets int) bool {
	/* convert port string from int to hex */
	portInt, err := strconv.Atoi(port)
	if err != nil {
		ELog.Println("Invalid port number:", port)
		return false
	}

	portHex := fmt.Sprintf("%04X", portInt)

	/* convert ip address from int to hex */
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false
	}

	ipv4 := ipAddr.To4()
	if ipv4 == nil {
		return false
	}

	ipParts := make([]string, 4)
	for i, b := range ipv4 {
		ipParts[3-i] = fmt.Sprintf("%02X", b)
	}
	ipHex := strings.Join(ipParts, "")

	/* get socket index nodes regarding this PID */
	fdPath := fmt.Sprintf("/proc/%d/fd/", pid)
	fdEntries, err := ioutil.ReadDir(fdPath)
	if err != nil {
		ELog.Printf("cannot read /proc/%d/fd, err: %s", pid, err)
		return false
	}

	var inodes []string
	for _, fdEntry := range fdEntries {
		link, err := os.Readlink(fdPath + fdEntry.Name())
		if err != nil || !strings.HasPrefix(link, "socket:") {
			continue
		}

		inode := strings.TrimPrefix(link, "socket:[")
		inode = strings.TrimSuffix(inode, "]")
		inodes = append(inodes, inode)
	}

	/* check tcp connection status */
	tcpPath := fmt.Sprintf("/proc/%d/net/tcp", pid)
	tcpFile, err := os.Open(tcpPath)
	if err != nil {
		ELog.Printf("Error opening %s, err %s", tcpPath, err)
		return false
	}
	defer tcpFile.Close()

	count := 0
	scanner := bufio.NewScanner(tcpFile)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 9 {
			continue
		}
		remAddress := fields[2]
		state := fields[3]
		entryInode := fields[9]

		/* "01" is the state code for ESTABLISHED */
		if remAddress == ipHex+":"+portHex && state == "01" {
			for _, inode := range inodes {
				if entryInode == inode {
					DLog.Println("isConnectionEstablished remAddress: ", remAddress, " entryInode: ", entryInode)
					count++
					break
				}
			}
		}
	}

	if numSockets != count {
		ELog.Printf("haven't ESTABLISHED socket %d < %d yet", count, numSockets)
		return false
	}

	return true
}

func waitForConnectionEstablished(pid int, ip string, port string, numSockets int, timeoutMs int) int {
	timeout := time.After(time.Duration(timeoutMs) * time.Millisecond)
	tick := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-timeout:
			ELog.Println("Timeout reached. Connection not established.")
			return -1
		case <-tick:
			if isConnectionEstablished(pid, ip, port, numSockets) {
				return 0
			}
		}
	}
}

func waitUnixSocketConnectable(socketPath string, pid int) int {
	const countMax = 10
	for count := 0; count < countMax; count++ {
		process := ucl.CheckProcessAlive(pid)
		if process == nil {
			ELog.Println("cannot find process: ", pid)
			return -1
		}

		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			WLog.Println("failed connect wayland server")
		} else {
			ILog.Println("success connection to wayland server")
			conn.Close()
			return 0
		}

		time.Sleep(100 * time.Millisecond)
	}
	ELog.Println("Timeout, Cannot connect wayland server: ", socketPath)
	return -1
}

func reserveRvgpuIndex() int {
	const CARD_MAX = 65
	const indexfile = "/tmp/rvgpu-index"
	for cardN := 0; cardN < CARD_MAX; cardN++ {
		devPath := fmt.Sprintf("/dev/dri/rvgpu_virtio%d", cardN)
		if !fileExists(devPath) {

			if fileExists(indexfile) {
				os.Remove(indexfile)
			}
			file, err := os.OpenFile(indexfile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
			if err != nil {
				ELog.Println("Error opening/creating file:", err)
				return -1
			}
			defer file.Close()

			indexString := fmt.Sprintf("%d\n", cardN)
			_, err = file.WriteString(indexString)
			if err != nil {
				ELog.Println("Error write index: ", err)
				return -1
			}
			return cardN
		}
	}
	ELog.Println("Cannot reserve index, Max card index: ", CARD_MAX)
	return -1
}

func lockFileExclusive(lockFile string) (*os.File, error) {
	file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		file.Close()
		return nil, err
	}

	return file, nil
}

func unlockFile(file *os.File) {
	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	file.Close()
}

func setExecCommSetting(cmd *exec.Cmd, envs []string) {
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = append(cmd.Env, envs...)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "%s [option] execComm\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "[option]\n")
	flag.PrintDefaults()
}

func main() {

	flag.Usage = printUsage

	var wg sync.WaitGroup
	var err error
	var ret int
	const (
		lockFile = "/tmp/rvgpu.lock"
	)

	var (
		verbose bool
		debug   bool
		screen  string
		targets ucl.MultiFlag
		appName string
	)

	flag.BoolVar(&verbose, "v", true, "verbose info log")
	flag.BoolVar(&debug, "d", false, "verbose debug log")

	flag.StringVar(&screen, "s", "1920x1080@0,0", "remote-virtio-gpu sender screen config")
	flag.Var(&targets, "n", "Specify multiple -n options (default 127.0.0.1:55667)")
	flag.StringVar(&appName, "appli_name", "", "specify application name")

	flag.Parse()

	if len(targets) == 0 {
		targets.Set("127.0.0.1:55667")
	}

	appArgs := flag.Args()
	if len(appArgs) == 0 {
		ELog.Println("no target app")
		os.Exit(1)
	}

	targetApp := appArgs[0]
	appOptions := appArgs[1:]

	var width, height int
	_, err = fmt.Sscanf(screen, "%dx%d", &width, &height)
	if err != nil {
		ELog.Println("cannot set rvgpu sender screen, err: ", err)
		os.Exit(1)
	}
	size := fmt.Sprintf("%dx%d", width, height)

	if verbose == true {
		ILog.SetOutput(os.Stderr)
	}

	if debug == true {
		DLog.SetOutput(os.Stderr)
	}

	SetLogPrefix("ucl-virtio-gpu-wl-send " + targetApp)

	lock, err := lockFileExclusive(lockFile)
	if err != nil {
		ELog.Printf("Unable to lock file: %v\n", err)
		os.Exit(1)
	}
	cardN := reserveRvgpuIndex()

	if cardN == -1 {
		ELog.Println("failed get rvgpu index")
		unlockFile(lock)
		os.Exit(1)
	}

	xdgRuntimeDir := ucl.GetEnv("XDG_RUNTIME_DIR", "/tmp")
	wlSocketName := fmt.Sprintf("rvgpu-wayland-%d", cardN)
	wlServerPath := xdgRuntimeDir + "/" + wlSocketName
	sigChan := make(chan os.Signal, 1)
	pidChan := make(chan int, 2)
	go ucl.SignalHandler(sigChan, pidChan, true)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM)

	var rvgpuProxyCmd, wlProxyCmd, rvgpuAppCmd *exec.Cmd
	var wlProxyEnv, rvgpuAppEnv []string
	rvgpuOptions := []string{"-s", screen}
	for _, target := range targets {
		rvgpuOptions = append(rvgpuOptions, "-n")
		rvgpuOptions = append(rvgpuOptions, target)
	}

	if appName != "" {
		rvgpuOptions = append(rvgpuOptions, "-i")
		rvgpuOptions = append(rvgpuOptions, appName)
	}

	rvgpuProxyCmd = exec.Command("rvgpu-proxy", rvgpuOptions...)

	setExecCommSetting(rvgpuProxyCmd, nil)

	err = rvgpuProxyCmd.Start()
	if err != nil {
		ELog.Println("cmd.Start fail:", err)
		goto WAIT_ALL
	}

	wg.Add(1)
	go ucl.WaitApp(rvgpuProxyCmd, &wg, "rvgpu-proxy")

	ret = waitRvgpuDevices(cardN, rvgpuProxyCmd.Process.Pid)
	if ret == -1 {
		ELog.Println("cannot find all rvgpu card and input devices")
		goto WAIT_ALL
	}
	unlockFile(lock)
	lock = nil

	for _, target := range targets {
		ip, port, err := net.SplitHostPort(target)
		if err != nil {
			ELog.Println("cannot set target ip adress and port, err: ", err)
			os.Exit(1)
		}
		/* rvgpu-proxy connect to rvgpu-renderer with two sockets (command and resource) */
		ret = waitForConnectionEstablished(rvgpuProxyCmd.Process.Pid, ip, port, 2, 1000)
		if ret == -1 {
			ELog.Println("cannot connect rvgpu sender and receiver by timeout")
			goto WAIT_ALL
		}
	}

	wlProxyCmd = exec.Command("rvgpu-wlproxy", "-s", size, "-S", wlSocketName, "-f")

	wlProxyEnv = append(wlProxyEnv, fmt.Sprintf("EGLWINSYS_DRM_DEV_NAME=/dev/dri/rvgpu_virtio%d", cardN))
	wlProxyEnv = append(wlProxyEnv, fmt.Sprintf("EGLWINSYS_DRM_TOUCH_DEV=/dev/input/rvgpu_touch%d", cardN))
	wlProxyEnv = append(wlProxyEnv, fmt.Sprintf("EGLWINSYS_DRM_MOUSE_DEV=/dev/input/rvgpu_mouse%d", cardN))
	wlProxyEnv = append(wlProxyEnv, fmt.Sprintf("EGLWINSYS_DRM_MOUSEABS_DEV=/dev/input/rvgpu_mouse_abs%d", cardN))
	wlProxyEnv = append(wlProxyEnv, fmt.Sprintf("EGLWINSYS_DRM_KEYBOARD_DEV=/dev/input/rvgpu_keyboard%d", cardN))

	wlProxyEnv = append(wlProxyEnv, "__GLX_VENDOR_LIBRARY_NAME=mesa")
	wlProxyEnv = append(wlProxyEnv, "MESA_LOADER_DRIVER_OVERRIDE=virtio_gpu")
	wlProxyEnv = append(wlProxyEnv, "XDG_RUNTIME_DIR="+xdgRuntimeDir)

	setExecCommSetting(wlProxyCmd, wlProxyEnv)

	err = wlProxyCmd.Start()
	if err != nil {
		ELog.Println("cmd.Start fail:", err)
		goto WAIT_ALL
	}
	wg.Add(1)
	go ucl.WaitApp(wlProxyCmd, &wg, "rvgpu-wlproxy")

	ret = waitUnixSocketConnectable(wlServerPath, wlProxyCmd.Process.Pid)
	if ret == -1 {
		ELog.Println("cannot connect wayland server: ", wlServerPath)
		goto WAIT_ALL
	}

	rvgpuAppCmd = exec.Command(targetApp, appOptions...)
	rvgpuAppEnv = append(rvgpuAppEnv, "__GLX_VENDOR_LIBRARY_NAME=mesa")
	rvgpuAppEnv = append(rvgpuAppEnv, "MESA_LOADER_DRIVER_OVERRIDE=virtio_gpu")
	rvgpuAppEnv = append(rvgpuAppEnv, "XDG_RUNTIME_DIR="+xdgRuntimeDir)
	rvgpuAppEnv = append(rvgpuAppEnv, "WAYLAND_DISPLAY="+wlSocketName)
	setExecCommSetting(rvgpuAppCmd, rvgpuAppEnv)

	err = rvgpuAppCmd.Start()
	if err != nil {
		ELog.Println("cmd.Start fail:", err)
		goto WAIT_ALL
	}
	wg.Add(1)
	go ucl.WaitApp(rvgpuAppCmd, &wg, targetApp)

WAIT_ALL:
	if lock != nil {
		unlockFile(lock)
	}

	/* Should kill the proceses before rvgpu-proxy to prevent zombie processes. */
	if rvgpuAppCmd != nil && rvgpuAppCmd.Process.Pid > 0 {
		pidChan <- rvgpuAppCmd.Process.Pid
	}
	if wlProxyCmd != nil && wlProxyCmd.Process.Pid > 0 {
		pidChan <- wlProxyCmd.Process.Pid
	}
	if rvgpuProxyCmd != nil && rvgpuProxyCmd.Process.Pid > 0 {
		pidChan <- rvgpuProxyCmd.Process.Pid
	}

	if err != nil || ret == -1 {
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}

	wg.Wait()
	ILog.Println("I'm finish")
}
