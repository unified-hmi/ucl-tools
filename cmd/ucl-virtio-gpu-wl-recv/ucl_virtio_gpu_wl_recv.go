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
	"flag"
	"fmt"
	"ucl-tools/internal/ucl"
	. "ucl-tools/internal/ulog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "%s [option]\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "[option]\n")
	flag.PrintDefaults()
}

func main() {

	flag.Usage = printUsage

	var wg sync.WaitGroup
	var err error
	var (
		verbose      bool
		debug        bool
		sfcId        string
		screen       string
		port         string
		initialColor string
		appId        string
	)

	flag.BoolVar(&verbose, "v", true, "verbose info log")
	flag.BoolVar(&debug, "d", false, "verbose debug log")

	flag.StringVar(&sfcId, "S", "9000", "specify remote-virtio-gpu reciever surface ID")
	flag.StringVar(&screen, "s", "1920x1080@0,0", "remote-virtio-gpu reciever screen config")
	flag.StringVar(&port, "P", "55667", "specify remote-virtio-gpu reciever port")
	flag.StringVar(&initialColor, "B", "0x33333333", "remote-virtio-gpu reciever color config")
	flag.StringVar(&appId, "a", "", "remote-virtio-gpu reciever application id")
	flag.Parse()

	xdgRuntimeDir := ucl.GetEnv("XDG_RUNTIME_DIR", "/run/user/1000")
	wlDisplay := ucl.GetEnv("WAYLAND_DISPLAY", "wayland-0")
	wlServerPath := xdgRuntimeDir + "/" + wlDisplay
	conn, err := net.Dial("unix", wlServerPath)
	if err != nil {
		ELog.Println("cannot connect wayland server: ", wlServerPath)
		os.Exit(1)
	} else {
		conn.Close()
	}

	sigChan := make(chan os.Signal, 1)
	pidChan := make(chan int, 2)
	go ucl.SignalHandler(sigChan, pidChan, true)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM)

	if verbose == true {
		ILog.SetOutput(os.Stderr)
	}

	if debug == true {
		DLog.SetOutput(os.Stderr)
	}

	SetLogPrefix("ucl-virtio-gpu-wl-recv")

	cmd := exec.Command("rvgpu-renderer", "-B", initialColor, "-p", port, "-b", screen, "-i", sfcId, "-a")

	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if appId != "" {
		env_app_id := "XDG_TOPLEVEL_FALLBACK_APP_ID=" + appId
		cmd.Env = append(cmd.Env, env_app_id)
	}

	err = cmd.Start()
	if err != nil {
		ELog.Println("cmd.Start fail:", err)
		/* kill my self */
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		goto WAIT_ALL
	}

	pidChan <- cmd.Process.Pid
	wg.Add(1)
	go ucl.WaitApp(cmd, &wg, "rvgpu-renderer")

WAIT_ALL:
	wg.Wait()
	ILog.Println("I'm finish")
}
