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

package ucl

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	. "ucl-tools/internal/ulog"
)

type ConsistencyProtocol struct {
	CommType string `json:"type"`
	CommData string `json:"data"`
}

func NcountMaster(
	numWorker int,
	sendNodeChans []chan []byte,
	waitNCountChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	ILog.Printf("(task=%s) Waiting for nCount from (%d) targets...", commTaskCtx.AppName, numWorker)

	connected := 0
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

LOOP:
	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ELog.Printf("(task=%s) NcountMaster close", commTaskCtx.AppName)
			return

		case <-waitNCountChan:
			connected++
			if connected >= numWorker {
				break LOOP
			}
		case <-t.C:
			ELog.Printf("(task=%s) WatchDog Worker is insufficient(%d < %d)", commTaskCtx.AppName, connected, numWorker)

			commTaskCtx.Cancel()
			return
		}
	}

	cp := ConsistencyProtocol{
		CommType: "ncount",
		CommData: "",
	}
	sendMsg, _ := json.Marshal(cp)
	for _, sendNodeChan := range sendNodeChans {
		sendNodeChan <- sendMsg
	}

	ILog.Printf("(task=%s) nCount completed for (%d) targets.", commTaskCtx.AppName, numWorker)
}

func ncountWorker(
	sendLcmChan chan []byte,
	waitNCountChan chan []byte,
	commTaskCtx *CommTaskContext) error {

	rand.Seed(time.Now().UnixNano())

	magicCode := commTaskCtx.AppName + strconv.Itoa(rand.Intn(1024))
	DLog.Println("magicCode: ", magicCode)

	cp := ConsistencyProtocol{
		CommType: "ncount",
		CommData: magicCode,
	}
	sendMsg, _ := json.Marshal(cp)
	sendLcmChan <- sendMsg

	select {
	case recvMsg := <-waitNCountChan:
		if !reflect.DeepEqual(recvMsg, sendMsg) {
			ILog.Printf("(task=%s) nCount reflect.DeepEqual ERR", commTaskCtx.AppName)
			commTaskCtx.Cancel()
			return errors.New(fmt.Sprintf("magic code mismatch"))
		}
	case <-commTaskCtx.Ctx.Done():
		ELog.Printf("(task=%s) ncountWorker close", commTaskCtx.AppName)
		return errors.New(fmt.Sprintf("other reason"))
	}

	ILog.Printf("(task=%s) nCount completed from Master", commTaskCtx.AppName)
	return nil
}

func NkeepMaster(
	sendNodeChan chan []byte,
	waitNKeepChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	retryInterval := 10 * time.Second
	timeoutInterval := 4 * time.Second

	sendt := time.NewTicker(retryInterval)
	recvt := time.NewTicker(timeoutInterval)
	defer sendt.Stop()
	defer recvt.Stop()

	cp := ConsistencyProtocol{
		CommType: "nkeep",
		CommData: "Check from Master",
	}
	sendMsg, _ := json.Marshal(cp)
	sendNodeChan <- sendMsg

	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ILog.Printf("(task=%s) NkeepMaster close", commTaskCtx.AppName)
			return

		case <-sendt.C:
			sendNodeChan <- sendMsg
			recvt = time.NewTicker(timeoutInterval)

		case <-recvt.C:
			ELog.Printf("(task=%s) NkeepMaster CheckAlive Timeout: No response", commTaskCtx.AppName)
			commTaskCtx.Cancel()
			return

		case <-waitNKeepChan:
			recvt.Stop()
			sendt = time.NewTicker(retryInterval)
		}
	}
}

func NkeepWorker(
	sendLcmChan chan []byte,
	waitNKeepChan chan int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) error {

	defer wg.Done()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown-Host"
	}

	cp := ConsistencyProtocol{
		CommType: "nkeep",
		CommData: "Response from " + commTaskCtx.AppName + " on " + hostname,
	}
	sendMsg, err := json.Marshal(cp)

	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ILog.Printf("(task=%s) NkeepWorker close", commTaskCtx.AppName)
			return nil

		case <-waitNKeepChan:
			sendLcmChan <- sendMsg
		}
	}

	return nil
}

func processExitHandler(
	pid int,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	for {
		select {
		case <-commTaskCtx.Ctx.Done():
			ILog.Printf("(task=%s) start ExitHandler for process(pid:%d) \n", commTaskCtx.AppName, pid)
			watchDogKill(pid, true)
			return
		}
	}
}

func waitProcess(
	cmd *exec.Cmd,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	pid := cmd.Process.Pid
	DLog.Printf("(task=%s) wait process(pid:%d)\n", commTaskCtx.AppName, pid)
	cmd.Wait()

	commTaskCtx.Cancel()
	ILog.Printf("(task=%s) finish process(pid:%d)\n", commTaskCtx.AppName, pid)
}

type ExecCommInfo struct {
	ExecComm    string
	ExecCommEnv []string
	IsWaitDeps  bool
}

func TimingWrapper(
	execCommInfo ExecCommInfo,
	sendLcmChan chan []byte,
	waitNCountChan chan []byte,
	commTaskCtx *CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	var subWg sync.WaitGroup
	var err error

	if execCommInfo.IsWaitDeps {
		err = ncountWorker(sendLcmChan, waitNCountChan, commTaskCtx)
		if err != nil {
			return
		}
	}

	cmd := strings.Split(execCommInfo.ExecComm, " ")
	appCmd := exec.Command(cmd[0], cmd[1:]...)
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	env := make([]string, len(os.Environ()))
	copy(env, os.Environ())
	appCmd.Env = env
	appCmd.Env = append(appCmd.Env, execCommInfo.ExecCommEnv...)
	err = appCmd.Start()
	if err != nil {
		ILog.Printf("(task=%s) appCmd Start error: ", commTaskCtx.AppName, err)
		commTaskCtx.Cancel()
		return
	}

	subWg.Add(1)
	go processExitHandler(appCmd.Process.Pid, commTaskCtx, &subWg)

	subWg.Add(1)
	go waitProcess(appCmd, commTaskCtx, &subWg)

	if !execCommInfo.IsWaitDeps {
		ncountWorker(sendLcmChan, waitNCountChan, commTaskCtx)
	}

	subWg.Wait()
	if execCommInfo.IsWaitDeps {
		ILog.Printf("(task=%s) timingSyncLaunch finish", commTaskCtx.AppName)
	} else {
		ILog.Printf("(task=%s) timingAsyncLaunch finish", commTaskCtx.AppName)
	}
}
