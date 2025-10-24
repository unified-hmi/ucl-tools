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

package dcmapi

import (
	"context"
	"encoding/json"
	"google.golang.org/grpc"
	"io/ioutil"
	"net"
	"os"
	"time"
	"ucl-tools/internal/ucl"
	. "ucl-tools/internal/ulog"
	"ucl-tools/proto/grpc/dcm"
	"unsafe"
)

type Callback func(unsafe.Pointer, unsafe.Pointer)

func uclLcmConnection() (dcm.DcmServiceClient, context.Context, error) {

	var err error
	var client dcm.DcmServiceClient
	var ctx context.Context
	vscrnDef, err := ucl.ReadVScrnDef()
	if err != nil {
		return client, ctx, err
	}

	targetAddr, err := ucl.GetLcmNodeAddr(vscrnDef)
	if err != nil {
		return client, ctx, err
	}

	targetIP, _, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return client, ctx, err
	}

	ipAddrs, err := ucl.GetIpv4AddrsOfAllInterfaces()
	if err != nil {
		ELog.Println("GetIpv4AddrsOfAllInterfaces error : ", err)
		return client, ctx, err
	}

	nodeId, err := ucl.GetMyNodeId(vscrnDef)
	var candidateIPs []string
	if err == nil {
		ipAddr, err := vscrnDef.GetIpAddrByNodeIdAndIpCandidateList(ipAddrs, nodeId)
		if err != nil {
			WLog.Println("GetIpAddrByNodeIdAndIpCandidateList error : ", err)
			candidateIPs = ipAddrs
		} else {
			candidateIPs = []string{ipAddr}
		}
	} else {
		candidateIPs = ipAddrs
	}

	if targetIP != "127.0.0.1" {
		filteredIPs := []string{}
		for _, ip := range candidateIPs {
			ipParsed := net.ParseIP(ip)
			if ipParsed != nil && ipParsed.IsLoopback() {
				continue
			}
			filteredIPs = append(filteredIPs, ip)
		}
		candidateIPs = filteredIPs
	}

	var conn *grpc.ClientConn
	var dialErr error

	for _, ip := range candidateIPs {
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			WLog.Printf("Invalid IP: %s, skipping", ip)
			continue
		}
		ILog.Printf("Trying to connect from IP: %s to target: %s", parsedIP, targetAddr)
		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP: parsedIP,
			},
			Timeout: 1 * time.Second,
		}

		conn, dialErr = grpc.DialContext(
			context.Background(),
			targetAddr,
			grpc.WithInsecure(),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", addr)
			}),
		)

		if dialErr == nil {
			ILog.Printf("Successfully connected from IP: %s", parsedIP)
			break
		} else {
			WLog.Printf("Connection from IP %s failed: %v", parsedIP, dialErr)
		}
	}
	if dialErr != nil {
		return client, ctx, dialErr
	}

	client = dcm.NewDcmServiceClient(conn)
	ctx = context.Background()

	return client, ctx, nil
}

func parseStatus(data string) int {

	switch data {
	case ucl.STAT_ExecBusy:
		return 2
	case ucl.STAT_ExecSuccess:
		return 1
	case ucl.STAT_ExecFin:
		return 0
	case ucl.STAT_ExecErr:
		return -1
	default:
		return -1
	}

}

func parseInfo(data string) int {

	switch data {
	case ucl.STAT_AppRunning:
		return 1
	case ucl.STAT_AppStop:
		return 0

	default:
		return -1
	}

}

func DcmClientGetExecutableAppList() []byte {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmGetExecutableAppList")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return nil
	}

	resp, err := dcmClient.DcmGetExecutableAppList(dcmCtx, &dcm.Empty{})
	if err != nil {
		return nil
	}

	retStatus := resp.GetStatus()
	retInfo := resp.GetInfo()
	ILog.Println("DcmGetExecutableAppList response:", retStatus)
	ILog.Println("DcmGetExecutableAppList info:", retInfo)

	return []byte(retInfo)

}

func DcmClientGetRunningAppList() []byte {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmGetRunningAppList")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return nil
	}

	resp, err := dcmClient.DcmGetRunningAppList(dcmCtx, &dcm.Empty{})
	if err != nil {
		return nil
	}

	retStatus := resp.GetStatus()
	retInfo := resp.GetInfo()
	ILog.Println("DcmGetRunningAppList response:", retStatus)
	ILog.Println("DcmGetRunningAppList info:", retInfo)

	return []byte(retInfo)

}

func DcmClientGetAppStatus(appName string) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmGetAppStatus: ", appName)

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	request := &dcm.AppControlRequest{
		AppName: appName,
	}

	resp, err := dcmClient.DcmGetAppStatus(dcmCtx, request)
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	retInfo := resp.GetInfo()
	ILog.Println("DcmGetAppStatus response:", retStatus)
	ILog.Println("DcmGetAppStatus info:", retInfo)

	return parseInfo(retInfo)

}

func DcmClientRunAppCommand(appCommandFilePath string) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmRunAppCommand: ", appCommandFilePath)

	f, err := os.Open(appCommandFilePath)
	if err != nil {
		return -1
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return -1
	}

	var checkData interface{}
	err = json.Unmarshal(data, &checkData)
	if err != nil {
		return -1
	}

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	request := &dcm.AppCommandRequest{
		AppJson: string(data),
	}

	resp, err := dcmClient.DcmRunAppCommand(dcmCtx, request)
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmRunAppCommand response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientRunApp(appName string) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmRunApp: ", appName)

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	request := &dcm.AppControlRequest{
		AppName: appName,
	}

	resp, err := dcmClient.DcmRunApp(dcmCtx, request)
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmRunApp response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientRunAppAsync(appName string) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmRunAppAsync: ", appName)

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	request := &dcm.AppControlRequest{
		AppName: appName,
	}

	resp, err := dcmClient.DcmRunAppAsync(dcmCtx, request)
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmRunAppAsync response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientRunAppAsyncCb(appName string, callback Callback, funcPointer unsafe.Pointer, argPointer unsafe.Pointer) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmRunAppAsyncCb: ", appName)

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	request := &dcm.AppControlRequest{
		AppName: appName,
	}

	resp, err := dcmClient.DcmRunApp(dcmCtx, request)
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmRunAppAsyncCb response:", retStatus)

	callback(funcPointer, argPointer)

	return parseStatus(retStatus)
}

func DcmClientStopApp(appName string) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmStopApp: ", appName)

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	request := &dcm.AppControlRequest{
		AppName: appName,
	}

	resp, err := dcmClient.DcmStopApp(dcmCtx, request)
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmStopApp response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientStopAppAll() int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmStopAppAll")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	resp, err := dcmClient.DcmStopAppAll(dcmCtx, &dcm.Empty{})
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmStopAppAll response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientLaunchCompositor() int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmLaunchCompositor")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	resp, err := dcmClient.DcmLaunchCompositor(dcmCtx, &dcm.Empty{})
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmLaunchCompositor response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientLaunchCompositorAsync() int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmLaunchCompositorAsync")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	resp, err := dcmClient.DcmLaunchCompositorAsync(dcmCtx, &dcm.Empty{})
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmLaunchCompositorAsync response:", retStatus)

	return parseStatus(retStatus)
}

func DcmClientLaunchCompositorAsyncCb(callback Callback, funcPointer unsafe.Pointer, argPointer unsafe.Pointer) int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmLaunchCompositorAsyncCb")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	resp, err := dcmClient.DcmLaunchCompositor(dcmCtx, &dcm.Empty{})
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmLaunchCompositorAsyncCb response:", retStatus)

	callback(funcPointer, argPointer)

	return parseStatus(retStatus)
}

func DcmClientStopCompositor() int {

	ILog.SetOutput(os.Stderr)
	ILog.Println("DcmStopCompositor")

	dcmClient, dcmCtx, err := uclLcmConnection()
	if err != nil {
		ELog.Println("UclLcmConnection: ", err)
		return -1
	}

	resp, err := dcmClient.DcmStopCompositor(dcmCtx, &dcm.Empty{})
	if err != nil {
		return -1
	}

	retStatus := resp.GetStatus()
	ILog.Println("DcmStopCompositor response:", retStatus)

	return parseStatus(retStatus)
}
