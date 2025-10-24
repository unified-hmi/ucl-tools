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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"ucl-tools/internal/ucl"
	. "ucl-tools/internal/ulog"
	"ucl-tools/proto/grpc/dcm"
)

var gVScrnDef *ucl.VScrnDef = nil

var commTaskCtxMu sync.Mutex
var commTaskCtxMap = make(map[string]*ucl.CommTaskContext)

func updateCommTaskCtxMap(appName string) (*ucl.CommTaskContext, error) {
	defer commTaskCtxMu.Unlock()

	commTaskCtxMu.Lock()
	if _, exists := commTaskCtxMap[appName]; exists || appName == "" {
		return nil, errors.New("ctxMap already exists: " + appName)
	}
	ctx := ucl.NewCommTaskCtx(appName)
	commTaskCtxMap[appName] = ctx

	return ctx, nil
}

func deleteCommTask(appName string) error {

	defer commTaskCtxMu.Unlock()

	commTaskCtxMu.Lock()
	if _, exists := commTaskCtxMap[appName]; !exists || appName == "" {
		return errors.New("ctxMap already exists: " + appName)
	}
	delete(commTaskCtxMap, appName)

	return nil
}

func cancelCommTaskCtx(appName string) error {

	commTaskCtxMu.Lock()
	commTaskCtx, exists := commTaskCtxMap[appName]
	commTaskCtxMu.Unlock()

	if !exists {
		return errors.New("no such ctx: " + appName)
	}
	commTaskCtx.Cancel()
	return nil
}

func CancelAllCommTaskCtxExceptCompositors() {
	defer commTaskCtxMu.Unlock()

	commTaskCtxMu.Lock()
	for appName, commTaskCtx := range commTaskCtxMap {
		if appName == "manageCompositors" {
			continue
		}
		commTaskCtx.Cancel()
	}
}

func isExistsCommTaskCtx(appName string) bool {

	commTaskCtxMu.Lock()
	_, exists := commTaskCtxMap[appName]
	commTaskCtxMu.Unlock()

	if !exists {
		return false
	}

	return true
}

func getRunningAppFromCommTaskCtxMap() []byte {

	defer commTaskCtxMu.Unlock()

	commTaskCtxMu.Lock()
	var appList []byte
	for appName := range commTaskCtxMap {
		if len(appList) > 0 {
			appList = append(appList, ',')
		}
		appList = append(appList, []byte(appName)...)
	}

	return appList
}

func isExistNode(dNodes []ucl.UclNode, chk ucl.UclNode) bool {
	for _, node := range dNodes {
		if node == chk {
			return true
		}
	}
	return false
}

func chkConnectableNode(node ucl.UclNode, cChan chan bool) {

	addr := node.Ip + ":" + strconv.Itoa(node.Port)

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		ILog.Printf("error:%s occured. %s not connectable", err, addr)

		cChan <- false
		return
	}

	cChan <- true

	conn.Close()
}

func getConnectableNode(dNodes []ucl.UclNode) (cNodes []ucl.UclNode) {

	numNodes := len(dNodes)

	cChans := make([]chan bool, numNodes)
	for i := range cChans {
		cChans[i] = make(chan bool, 1)
	}

	for i := 0; i < numNodes; i++ {
		go chkConnectableNode(dNodes[i], cChans[i])
	}

	for i := 0; i < numNodes; i++ {
		connectable := <-cChans[i]
		if connectable {
			cNodes = append(cNodes, dNodes[i])
		}
	}

	DLog.Printf("dNodes %v", dNodes)
	DLog.Printf("cNodes %v", cNodes)

	return
}

func removeDisconnectNode(mJson map[string]interface{}, cNodes []ucl.UclNode) error {

	switch mJson["format_v1"].(map[string]interface{})["command_type"] {
	case "remote_virtio_gpu", "transport", "launch_compositors":
		if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
			sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
			chk := sender["launcher"].(ucl.UclNode)
			if !isExistNode(cNodes, chk) {
				return errors.New("sender is not connectable")
			}
		}

		var Recvs []interface{}

		if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
			receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
			for _, r := range receivers {
				chk := r.(map[string]interface{})["launcher"].(ucl.UclNode)
				if isExistNode(cNodes, chk) {
					Recvs = append(Recvs, r)
				}
			}

			if len(Recvs) == 0 {
				return errors.New("not exist connectable receivers")
			}

			mJson["format_v1"].(map[string]interface{})["receivers"] = Recvs
		}

	case "local":
		local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
		chk := local["launcher"].(ucl.UclNode)
		if !isExistNode(cNodes, chk) {
			return errors.New("local is not connectable")
		}

	default:
		return errors.New("command type err")
	}

	return nil
}

func getDistribNodeAddr(mJson map[string]interface{}) string {

	var node ucl.UclNode

	if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
		sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
		node = sender["launcher"].(ucl.UclNode)

	} else if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
		receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
		node = receivers[0].(map[string]interface{})["launcher"].(ucl.UclNode)

	} else if mJson["format_v1"].(map[string]interface{})["local"] != nil {
		local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
		node = local["launcher"].(ucl.UclNode)

	} else {
		ELog.Printf("command_type: %s is not supported \n", mJson["format_v1"].(map[string]interface{})["command_type"])
		return ""
	}

	return node.Ip + ":" + strconv.Itoa(node.Port)
}

func getDistribNode(mJson map[string]interface{}) (dNodes []ucl.UclNode) {

	switch mJson["format_v1"].(map[string]interface{})["command_type"] {
	case "remote_virtio_gpu", "transport", "launch_compositors":
		if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
			sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
			dNodes = append(dNodes, sender["launcher"].(ucl.UclNode))
		}

		if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
			receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
			for _, r := range receivers {
				chk := r.(map[string]interface{})["launcher"].(ucl.UclNode)
				if !isExistNode(dNodes, chk) {
					dNodes = append(dNodes, chk)
				}
			}
		}

	case "local":
		if mJson["format_v1"].(map[string]interface{})["local"] != nil {
			local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
			dNodes = append(dNodes, local["launcher"].(ucl.UclNode))
		}

	default:
		ELog.Printf("command_type: %s is not supported \n", mJson["format_v1"].(map[string]interface{})["command_type"])
	}

	return
}

func splitCommandforEachNode(mJson map[string]interface{}) ([]map[string]interface{}, int) {

	var output []map[string]interface{}

	if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
		senderObj := map[string]interface{}{
			"format_v1": map[string]interface{}{
				"command_type": mJson["format_v1"].(map[string]interface{})["command_type"],
				"appli_name":   mJson["format_v1"].(map[string]interface{})["appli_name"],
				"sender":       mJson["format_v1"].(map[string]interface{})["sender"],

				/* The sender needs the receiver's IP:port status. */
				"receivers": mJson["format_v1"].(map[string]interface{})["receivers"],
			},
		}

		output = append(output, senderObj)
	}

	if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
		receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
		for _, receiver := range receivers {
			/*
				If a receiver needs to be launched as a compositor,
				it should not be launched in any sequence other than the launch_compositors command.
			*/
			if mJson["format_v1"].(map[string]interface{})["command_type"] != "launch_compositors" {
				backendParams := receiver.(map[string]interface{})["backend_params"].(map[string]interface{})
				listenPort := int(backendParams["listen_port"].(float64))
				isComp := ucl.IsRecvCompositor(listenPort, gVScrnDef)
				if isComp == true {
					continue
				}
			}

			receiverObj := map[string]interface{}{
				"format_v1": map[string]interface{}{
					"command_type": mJson["format_v1"].(map[string]interface{})["command_type"],
					"appli_name":   mJson["format_v1"].(map[string]interface{})["appli_name"],
					"receivers":    []interface{}{receiver},
				},
			}

			output = append(output, receiverObj)
		}
	}

	if mJson["format_v1"].(map[string]interface{})["local"] != nil {
		localObj := map[string]interface{}{
			"format_v1": map[string]interface{}{
				"command_type": mJson["format_v1"].(map[string]interface{})["command_type"],
				"appli_name":   mJson["format_v1"].(map[string]interface{})["appli_name"],
				"local":        mJson["format_v1"].(map[string]interface{})["local"],
			},
		}
		output = append(output, localObj)
	}

	return output, len(output)
}

func expandAliasNode(mJson map[string]interface{}, alias map[string]ucl.UclNode) {
	/* expand alias */
	var key string

	switch mJson["format_v1"].(map[string]interface{})["command_type"] {
	case "remote_virtio_gpu", "transport", "launch_compositors":
		if mJson["format_v1"].(map[string]interface{})["sender"] != nil {
			sender := mJson["format_v1"].(map[string]interface{})["sender"].(map[string]interface{})
			switch sender["launcher"].(type) {
			case string:
				key = sender["launcher"].(string)
				sender["launcher"] = alias[key]
			}
		}

		if mJson["format_v1"].(map[string]interface{})["receivers"] != nil {
			receivers := mJson["format_v1"].(map[string]interface{})["receivers"].([]interface{})
			for _, r := range receivers {
				receiver := r.(map[string]interface{})
				switch receiver["launcher"].(type) {
				case string:
					key = receiver["launcher"].(string)
					receiver["launcher"] = alias[key]
				}
			}
		}
	case "local":
		local := mJson["format_v1"].(map[string]interface{})["local"].(map[string]interface{})
		switch local["launcher"].(type) {
		case string:
			key = local["launcher"].(string)
			local["launcher"] = alias[key]
		}

	default:
		ELog.Printf("command_type: %s is not supported \n", mJson["format_v1"].(map[string]interface{})["command_type"])
	}
}

func handleNodeConnection(
	addr string,
	command string,
	sendNodeChan chan []byte,
	waitNCountChan chan int,
	commTaskCtx *ucl.CommTaskContext,
	wg *sync.WaitGroup) {

	defer wg.Done()

	conn, err := ucl.ConnectTarget(addr)
	if err != nil {
		ELog.Printf("(task=%s) ConnectTarget : %s\n", commTaskCtx.AppName, err)
		return
	}
	ILog.Printf("(task=%s) Dial connected to %s", commTaskCtx.AppName, addr)
	defer conn.Close()

	err = ucl.SendCommand(conn, ucl.CMD_DistribComm, command)
	if err != nil {
		ELog.Printf("(task=%s) sendCommand : %s\n", commTaskCtx.AppName, err)
		return
	}

	err = ucl.WaitMagicCode(conn)
	if err != nil {
		ELog.Printf("(task=%s) WaitMagicCode : %s\n", commTaskCtx.AppName, err)
		return
	}

	var subWg sync.WaitGroup
	waitNKeepChan := make(chan int, 1)
	subWg.Add(1)
	go ucl.NkeepMaster(sendNodeChan, waitNKeepChan, commTaskCtx, &subWg)

	/* ucl-node connection loop */
	rcvNodeChan := make(chan []byte, 2)
	go ucl.ConnReadLoop(conn, rcvNodeChan)

	var respNCountMsg []byte
	var cp ucl.ConsistencyProtocol
LOOP:
	for {
		select {
		case sendMsg := <-sendNodeChan:
			json.Unmarshal(sendMsg, &cp)
			if cp.CommType == "ncount" {
				sendMsg = respNCountMsg
			}
			err = ucl.ConnWriteWithSize(conn, sendMsg)
			if err != nil {
				ELog.Printf("(task=%s) ERR ConnWriteWithSize : %s\n", commTaskCtx.AppName, err)
				break LOOP
			}

		case recvMsg := <-rcvNodeChan:
			if recvMsg != nil {
				DLog.Printf("recv %s\n", recvMsg)
				json.Unmarshal(recvMsg, &cp)
				if cp.CommType == "ncount" {
					respNCountMsg = recvMsg
					waitNCountChan <- 1
				} else if cp.CommType == "nkeep" {
					waitNKeepChan <- 1
				} else {
					break LOOP
				}
			} else {
				ILog.Printf("(task=%s) Disconnected from the Node side", commTaskCtx.AppName)
				commTaskCtx.Cancel()
				break LOOP
			}
		case <-commTaskCtx.Ctx.Done():
			break LOOP
		}
	}

	subWg.Wait()
}

func generateLaunchCompsitorsComm() []byte {

	rvgpucompath := ucl.GetEnv("RVGPU_LAUNCH_COMM_PATH", "/usr/bin/")
	if !strings.HasSuffix(rvgpucompath, "/") {
		rvgpucompath += "/"
	}
	command := rvgpucompath + "ucl-virtio-gpu-rvgpu-compositor"

	var receivers []interface{}
	for _, fwn := range gVScrnDef.DistributedWindowSystem.FrameworkNode {
		for _, com := range fwn.Compositor {
			NodeId := fwn.NodeId
			VDisplayId := com.VDisplayIds[0]

			var launcherName string
			for _, node := range gVScrnDef.Nodes {
				if NodeId == node.NodeId {
					launcherName = node.HostName
					break
				}
			}
			var scanx, scany, scanw, scanh int
			for _, rdisplay := range gVScrnDef.RealDisplays {
				if NodeId == rdisplay.NodeId && VDisplayId == rdisplay.VDisplayId {
					scanx = 0
					scany = 0
					scanw = rdisplay.PixelW
					scanh = rdisplay.PixelH
				}
			}

			backendParams := map[string]interface{}{
				"scanout_x":            scanx,
				"scanout_y":            scany,
				"scanout_w":            scanw,
				"scanout_h":            scanh,
				"initial_screen_color": "0x33333333",
			}
			if com.IviSurfaceId > 0 {
				backendParams["ivi_surface_id"] = com.IviSurfaceId
			}
			if com.SockDomainName != "" {
				backendParams["sock_domain_name"] = com.SockDomainName
			}
			if com.ListenPort > 0 {
				backendParams["listen_port"] = com.ListenPort
			} else {
				return nil
			}

			receiver := map[string]interface{}{
				"launcher":       launcherName,
				"command":        command,
				"env":            "",
				"backend_params": backendParams,
			}

			receivers = append(receivers, receiver)
		}
	}

	obj := map[string]interface{}{
		"format_v1": map[string]interface{}{
			"appli_name":   "manageCompositors",
			"command_type": "launch_compositors",
			"receivers":    receivers,
		},
	}

	modJsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil
	}

	return modJsonBytes
}

func getAppInfoFromNode(
	addr string, data string, comm string,
	rcvDataCh chan []byte) {

	DLog.Printf("getAppInfoFromNode targetAddr: %s \n", addr)
	conn, err := ucl.ConnectTarget(addr)
	if err != nil {
		ELog.Printf("getAppInfo ConnectTarget : %s\n", err)
		return
	}
	ILog.Println("getAppInfo Dial connected to ", addr)
	defer conn.Close()

	err = ucl.SendCommand(conn, comm, data)
	if err != nil {
		ELog.Printf("getAppInfo SendCommand : %s\n", err)
		return
	}

	//wait for message from Node.
	readBuf, err := ucl.ConnReadWithSize(conn)
	if err != nil {
		DLog.Printf("Termination from launcer : %s \n", err)
		rcvDataCh <- nil
	} else {
		DLog.Printf("resp from Node: %s \n", readBuf)
		rcvDataCh <- readBuf
	}

}

func getAppCmdFromEachNode(appName string) []byte {

	fNodes := ucl.GetFrameworkNode(gVScrnDef)
	DLog.Println("GetFrameworkNode ", fNodes)

	cNodes := getConnectableNode(fNodes)
	rcvDataCh := make(chan []byte, len(cNodes))
	for _, fnip := range cNodes {
		targetAddr := fnip.Ip + ":" + strconv.Itoa(fnip.Port)
		go getAppInfoFromNode(targetAddr, appName, ucl.CMD_GetAppComm, rcvDataCh)
	}

	var appCommand []byte
	for i := 0; i < len(cNodes); i++ {
		select {
		case rcvData := <-rcvDataCh:
			if rcvData != nil {
				appCommand = rcvData
			}
		}
	}

	return appCommand
}

func getAppCmd(appName string) []byte {

	var command []byte
	if appName == "manageCompositors" {
		command = generateLaunchCompsitorsComm()
		if command == nil {
			ELog.Printf("generateLaunchCompsitorsComm error")
			return nil
		}
	} else {
		command = getAppCmdFromEachNode(appName)
		if command == nil {
			ELog.Printf("getAppCmdFromEachNode error")
			return nil
		}
	}

	return command

}

func getAppListFromEachNode() []byte {

	fNodes := ucl.GetFrameworkNode(gVScrnDef)
	DLog.Println("GetFrameworkNode ", fNodes)

	cNodes := getConnectableNode(fNodes)
	data, _ := json.Marshal(cNodes)
	rcvDataCh := make(chan []byte, len(cNodes))
	for _, fnip := range cNodes {
		targetAddr := fnip.Ip + ":" + strconv.Itoa(fnip.Port)
		go getAppInfoFromNode(targetAddr, string(data), ucl.CMD_GetExecutableAppList, rcvDataCh)
	}

	var appList []byte
	for i := 0; i < len(cNodes); i++ {
		select {
		case rcvData := <-rcvDataCh:
			if rcvData == nil {
				continue
			}
			if len(appList) > 0 {
				appList = append(appList, ',')
			}
			appList = append(appList, rcvData...)
		}
	}

	//Remove duplicate information
	if appList != nil {
		parts := bytes.Split(appList, []byte{','})
		seen := make(map[string]struct{}, len(parts))
		var out [][]byte
		for _, p := range parts {
			key := string(p)
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				out = append(out, p)
			}
		}
		appList = bytes.Join(out, []byte{','})
	}

	return appList
}

func dispatchCommToNodes(command []byte, commTaskCtx *ucl.CommTaskContext) {

	defer deleteCommTask(commTaskCtx.AppName)

	mJson := make(map[string]interface{})
	err := json.Unmarshal(command, &mJson)
	if err != nil {
		ELog.Printf("Unmarshal json command error: %s \n", err)
		commTaskCtx.Cancel()
		return
	}

	aip := ucl.ReadAliasIp(gVScrnDef)
	expandAliasNode(mJson, aip)

	dNodes := getDistribNode(mJson)
	cNodes := getConnectableNode(dNodes)

	err = removeDisconnectNode(mJson, cNodes)
	if err != nil {
		ELog.Printf("removeDisconnectNode: %s \n", err)
		commTaskCtx.Cancel()
		return
	}
	mJsonList, targetNum := splitCommandforEachNode(mJson)

	waitNCountChan := make(chan int, targetNum)
	sendNodeChans := make([]chan []byte, targetNum)
	for i := range sendNodeChans {
		sendNodeChans[i] = make(chan []byte, 1)
	}

	var subWg sync.WaitGroup
	subWg.Add(1)
	go ucl.NcountMaster(targetNum, sendNodeChans, waitNCountChan, commTaskCtx, &subWg)

	for i, mJson := range mJsonList {
		targetAddr := getDistribNodeAddr(mJson)
		newCommand, _ := json.Marshal(&mJson)
		subWg.Add(1)
		go handleNodeConnection(targetAddr, string(newCommand), sendNodeChans[i], waitNCountChan, commTaskCtx, &subWg)
	}

	subWg.Wait()
}

func getAppName(data []byte) string {
	mJson := make(map[string]interface{})
	err := json.Unmarshal(data, &mJson)
	if err != nil {
		ELog.Printf("Unmarshal json command error: %s \n", err)
		return ""
	}

	if mJson["format_v1"].(map[string]interface{})["appli_name"] != nil {
		return mJson["format_v1"].(map[string]interface{})["appli_name"].(string)
	}

	return ""
}

type server struct {
	dcm.UnimplementedDcmServiceServer
}

func (s *server) DcmGetExecutableAppList(grpcCtx context.Context, req *dcm.Empty) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr
	infoData := ""

	appList := getAppListFromEachNode()
	if appList != nil {
		responseStatus = ucl.STAT_ExecFin
		infoData = string(appList)
	}

	return &dcm.Response{Status: responseStatus, Info: infoData}, nil
}

func (s *server) DcmGetRunningAppList(grpcCtx context.Context, req *dcm.Empty) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr
	infoData := ""

	appList := getRunningAppFromCommTaskCtxMap()
	if appList != nil {
		responseStatus = ucl.STAT_ExecFin
		infoData = string(appList)
	}

	return &dcm.Response{Status: responseStatus, Info: infoData}, nil
}

func (s *server) DcmGetAppStatus(grpcCtx context.Context, req *dcm.AppControlRequest) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecFin
	responseInfo := ucl.STAT_AppStop

	appName := req.GetAppName()
	taskName := appName
	if exists := isExistsCommTaskCtx(taskName); exists {
		responseInfo = ucl.STAT_AppRunning
	}

	return &dcm.Response{Status: responseStatus, Info: responseInfo}, nil
}

func (s *server) DcmRunAppCommand(grpcCtx context.Context, req *dcm.AppCommandRequest) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr

	appJson := req.GetAppJson()
	appName := getAppName([]byte(appJson))
	if appName == "" {
		ELog.Printf("json command error")
		return &dcm.Response{Status: responseStatus}, nil
	}
	taskName := appName

	commTaskCtx, err := updateCommTaskCtxMap(taskName)
	if err != nil {
		responseStatus = ucl.STAT_ExecBusy
		return &dcm.Response{Status: responseStatus}, nil
	}

	command := []byte(appJson)
	if command == nil {
		ELog.Printf("No json command error")
		deleteCommTask(taskName)
		return &dcm.Response{Status: responseStatus}, nil
	}

	go dispatchCommToNodes(command, commTaskCtx)

	select {
	case <-commTaskCtx.Ctx.Done():
		responseStatus = ucl.STAT_ExecFin

	case <-grpcCtx.Done():
		responseStatus = ucl.STAT_ExecFin
		commTaskCtx.Cancel()
	}

	return &dcm.Response{Status: responseStatus}, nil

}

func (s *server) DcmRunApp(grpcCtx context.Context, req *dcm.AppControlRequest) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr

	appName := req.GetAppName()
	taskName := appName
	commTaskCtx, err := updateCommTaskCtxMap(taskName)
	if err != nil {
		responseStatus = ucl.STAT_ExecBusy
		return &dcm.Response{Status: responseStatus}, nil
	}

	command := getAppCmd(appName)
	if command == nil {
		ELog.Printf("(task=%s) json command not found", taskName)
		deleteCommTask(taskName)
		return &dcm.Response{Status: responseStatus}, nil
	}

	go dispatchCommToNodes(command, commTaskCtx)

	select {
	case <-commTaskCtx.Ctx.Done():
		responseStatus = ucl.STAT_ExecFin

	case <-grpcCtx.Done():
		responseStatus = ucl.STAT_ExecFin
		commTaskCtx.Cancel()
	}

	return &dcm.Response{Status: responseStatus}, nil
}

func (s *server) DcmRunAppAsync(grpcCtx context.Context, req *dcm.AppControlRequest) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecSuccess

	appName := req.GetAppName()
	taskName := appName
	commTaskCtx, err := updateCommTaskCtxMap(taskName)
	if err != nil {
		responseStatus = ucl.STAT_ExecBusy
		return &dcm.Response{Status: responseStatus}, nil
	}

	command := getAppCmd(appName)
	if command == nil {
		ELog.Printf("(task=%s) json command not found", taskName)
		deleteCommTask(taskName)
		return &dcm.Response{Status: responseStatus}, nil
	}

	go dispatchCommToNodes(command, commTaskCtx)

	return &dcm.Response{Status: responseStatus}, nil

}

func (s *server) DcmStopApp(grpcCtx context.Context, req *dcm.AppControlRequest) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr

	taskName := req.GetAppName()

	if err := cancelCommTaskCtx(taskName); err == nil {
		responseStatus = ucl.STAT_ExecFin
	}

	return &dcm.Response{Status: responseStatus}, nil
}

func (s *server) DcmStopAppAll(grpcCtx context.Context, req *dcm.Empty) (*dcm.Response, error) {

	CancelAllCommTaskCtxExceptCompositors()
	responseStatus := ucl.STAT_ExecFin

	return &dcm.Response{Status: responseStatus}, nil
}

func (s *server) DcmLaunchCompositor(grpcCtx context.Context, req *dcm.Empty) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr

	appName := "manageCompositors"
	taskName := appName
	commTaskCtx, err := updateCommTaskCtxMap(taskName)
	if err != nil {
		responseStatus = ucl.STAT_ExecBusy
		return &dcm.Response{Status: responseStatus}, nil
	}

	command := getAppCmd(appName)
	if command == nil {
		ELog.Printf("(task=%s) json command not found", taskName)
		deleteCommTask(taskName)
		return &dcm.Response{Status: responseStatus}, nil
	}

	go dispatchCommToNodes(command, commTaskCtx)

	select {
	case <-commTaskCtx.Ctx.Done():
		responseStatus = ucl.STAT_ExecFin

	case <-grpcCtx.Done():
		responseStatus = ucl.STAT_ExecFin
		commTaskCtx.Cancel()
	}

	return &dcm.Response{Status: responseStatus}, nil
}

func (s *server) DcmLaunchCompositorAsync(grpcCtx context.Context, req *dcm.Empty) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecSuccess

	appName := "manageCompositors"
	taskName := appName
	commTaskCtx, err := updateCommTaskCtxMap(taskName)
	if err != nil {
		responseStatus = ucl.STAT_ExecBusy
		return &dcm.Response{Status: responseStatus}, nil
	}

	command := getAppCmd(appName)
	if command == nil {
		ELog.Printf("(task=%s) json command not found", taskName)
		deleteCommTask(taskName)
		return &dcm.Response{Status: responseStatus}, nil
	}

	go dispatchCommToNodes(command, commTaskCtx)

	return &dcm.Response{Status: responseStatus}, nil
}

func (s *server) DcmStopCompositor(grpcCtx context.Context, req *dcm.Empty) (*dcm.Response, error) {

	responseStatus := ucl.STAT_ExecErr
	taskName := "manageCompositors"

	if err := cancelCommTaskCtx(taskName); err == nil {
		responseStatus = ucl.STAT_ExecFin
	}

	return &dcm.Response{Status: responseStatus}, nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, " %s [OPTIONS]\n\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "Options\n")
	flag.PrintDefaults()
}

func main() {

	flag.Usage = printUsage

	var (
		verbose      bool
		debug        bool
		vScrnDefFile string
		err          error
	)

	flag.BoolVar(&verbose, "v", true, "verbose info log")
	flag.BoolVar(&debug, "d", false, "verbose debug log")
	flag.StringVar(&vScrnDefFile, "f", ucl.VSCRNDEF_FILE, "virtual-screen-def.json file Path")
	flag.Parse()

	if verbose == true {
		ILog.SetOutput(os.Stderr)
	}

	if debug == true {
		DLog.SetOutput(os.Stderr)
	}

	gVScrnDef, err = ucl.ReadVScrnDef(vScrnDefFile)
	if err != nil {
		ELog.Println("ReadVScrnDef error : ", err)
		os.Exit(1)
	}

	listenAddr, err := ucl.GetLcmNodeAddr(gVScrnDef)
	if err != nil {
		ELog.Println("GetLcmNodeAddr error : ", err)
		os.Exit(1)
	}

	ILog.Printf("listenAddr=%s", listenAddr)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		ELog.Printf("Listen error: %s", err)
		os.Exit(1)
	}
	defer listener.Close()

	s := grpc.NewServer()
	dcm.RegisterDcmServiceServer(s, &server{})
	ILog.Println("gRPC Server listening on ", listenAddr)
	if err := s.Serve(listener); err != nil {
		ELog.Printf("Failed to serve: %v", err)
		os.Exit(1)
	}
}
