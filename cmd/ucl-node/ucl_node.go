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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"ucl-tools/internal/ucl"
	. "ucl-tools/internal/ulog"
)

type frontendParams_t struct {
	ScanOutX       int `json:"scanout_x"`
	ScanOutY       int `json:"scanout_y"`
	ScanOutW       int `json:"scanout_w"`
	ScanOutH       int `json:"scanout_h"`
	ServerPort     int `json:"server_port"`
	SessionTimeOut int `json:"session_timeout"`
}

type backendParams_t struct {
	IviSurfaceId       *int    `json:"ivi_surface_id"`
	SockDomainName     *string `json:"sock_domain_name"`
	ScanOutX           int     `json:"scanout_x"`
	ScanOutY           int     `json:"scanout_y"`
	ScanOutW           int     `json:"scanout_w"`
	ScanOutH           int     `json:"scanout_h"`
	ListenPort         int     `json:"listen_port"`
	InitialScreenColor string  `json:"initial_screen_color"`
}

type qtEnv_t struct {
	IviSurfaceId int  `json:"ivi_surface_id"`
	IviLayerId   *int `json:"ivi_layer_id"`
}

type sender_t struct {
	Launcher       ucl.UclNode      `json:"launcher"`
	Command        string           `json:"command"`
	FrontendParams frontendParams_t `json:"frontend_params"`
	Env            string           `json:"env"`
	QtEnv          *qtEnv_t         `json:"qt_env"`
	Appli          string           `json:"appli"`
}

type receiver_t struct {
	Launcher      ucl.UclNode     `json:"launcher"`
	Command       string          `json:"command"`
	BackendParams backendParams_t `json:"backend_params"`
	Env           string          `json:"env"`
}

type local_t struct {
	Launcher ucl.UclNode `json:"launcher"`
	Command  string      `json:"command"`
	Env      string      `json:"env"`
	Appli    string      `json:"appli"`
}

type uclCommand struct {
	FormatV1 struct {
		AppName     string       `json:"appli_name"`
		CommandType string       `json:"command_type"`
		Sender      sender_t     `json:"sender"`
		Receivers   []receiver_t `json:"receivers"`
		Local       local_t      `json:"local"`
	} `json:"format_v1"`
}

func launcherExists(launcher string, uclNodes []ucl.UclNode) bool {
	for _, node := range uclNodes {
		if launcher == node.HostName {
			return true
		}
	}
	return false
}

func validateAppInfo(mJson map[string]interface{}, nodes []ucl.UclNode) bool {
	fmt, ok := mJson["format_v1"].(map[string]interface{})
	if !ok {
		return false
	}
	cmd, ok := fmt["command_type"].(string)
	if !ok {
		return false
	}

	switch cmd {
	case "remote_virtio_gpu", "transport":
		sender, ok := fmt["sender"].(map[string]interface{})
		if !ok || !launcherExists(sender["launcher"].(string), nodes) {
			return false
		}
		receivers, ok := fmt["receivers"].([]interface{})
		if !ok {
			return false
		}
		for _, recv := range receivers {
			name := recv.(map[string]interface{})["launcher"].(string)
			if !launcherExists(name, nodes) {
				return false
			}
		}
		return true

	case "local":
		l, ok := fmt["local"].(map[string]interface{})
		return ok && launcherExists(l["launcher"].(string), nodes)

	default:
		return false
	}
}

func getExecutableAppList(recvData []byte) (string, error) {

	var nodes []ucl.UclNode
	if err := json.Unmarshal(recvData, &nodes); err != nil {
		return "", err
	}

	dcmpath := ucl.GetEnv("DCMPATH", "/var/local/uhmi-app/")
	if !strings.HasSuffix(dcmpath, "/") {
		dcmpath += "/"
	}
	dirs, err := ioutil.ReadDir(dcmpath)
	if err != nil {
		return "", err
	}

	var hitDirs []string
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		dirName := dir.Name()
		fname := filepath.Join(dcmpath, dirName, "app.json")

		jsonBytes, err := ioutil.ReadFile(fname)
		if err != nil {
			continue
		}
		var mJson map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &mJson); err != nil {
			continue
		}
		if !validateAppInfo(mJson, nodes) {
			continue
		}

		hitDirs = append(hitDirs, dirName)
	}

	if len(hitDirs) > 0 {
		DLog.Printf("hitDirs %s", strings.Join(hitDirs, ","))
		return strings.Join(hitDirs, ","), nil
	} else {
		return "", errors.New(fmt.Sprintf("App.json not found"))
	}
}

func ReadAppComm(appName string) ([]byte, error) {

	dcmpath := ucl.GetEnv("DCMPATH", "/var/local/uhmi-app/")
	if !strings.HasSuffix(dcmpath, "/") {
		dcmpath += "/"
	}
	fname := dcmpath + appName + "/app.json"

	jsonBytes, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

func makeQtCmdParams(uclComm uclCommand, makeParam string) string {
	/* Qt Env params*/
	if uclComm.FormatV1.Sender.QtEnv != nil {
		qenv := uclComm.FormatV1.Sender.QtEnv
		makeParam = makeParam + " -S " + strconv.Itoa(qenv.IviSurfaceId)
		if qenv.IviLayerId != nil {
			makeParam = makeParam + " -L " + strconv.Itoa(*qenv.IviLayerId)
		}
	}
	return makeParam
}

func makeSenderCmdParams(uclComm uclCommand, makeParam string) string {

	/* frontend params*/
	rparam := uclComm.FormatV1.Sender.FrontendParams
	makeParam = makeParam + " -s " +
		strconv.Itoa(rparam.ScanOutW) + "x" + strconv.Itoa(rparam.ScanOutH) + "@" +
		strconv.Itoa(rparam.ScanOutX) + "," + strconv.Itoa(rparam.ScanOutY)

	/* receiver ip addrs*/
	for _, r := range uclComm.FormatV1.Receivers {
		makeParam = makeParam + " -n " + r.Launcher.Ip + ":" + strconv.Itoa(r.BackendParams.ListenPort)
	}

	/* App params */
	makeParam = makeParam + " --appli_name " + uclComm.FormatV1.AppName
	makeParam = makeParam + " " + uclComm.FormatV1.Sender.Appli

	return makeParam
}

func makeLocalCmdParams(uclComm uclCommand, makeParam string) string {

	/* App params */
	makeParam = makeParam + " " + uclComm.FormatV1.Local.Appli

	return makeParam
}

func makeReceiverCmdParams(uclComm uclCommand, makeParam string, r receiver_t) string {

	/* backend params */
	rparam := r.BackendParams

	makeParam = makeParam + " -s " +
		strconv.Itoa(rparam.ScanOutW) + "x" + strconv.Itoa(rparam.ScanOutH) + "@" +
		strconv.Itoa(rparam.ScanOutX) + "," + strconv.Itoa(rparam.ScanOutY) +
		" -P " + strconv.Itoa(rparam.ListenPort) +
		" -B " + rparam.InitialScreenColor

	if rparam.IviSurfaceId != nil {
		makeParam = makeParam + " -S " + strconv.Itoa(*rparam.IviSurfaceId)
	}
	if rparam.SockDomainName != nil {
		makeParam = makeParam + " -D " + *rparam.SockDomainName
	}

	return makeParam
}

func convertEnvVars(envString string) []string {
	var envSegments []string
	insideSingleQuotes := false
	insideDoubleQuotes := false
	parseType := "key"
	envKey := ""
	envValue := ""

	currentSegment := ""

	if len(envString) == 0 {
		return envSegments
	}

	for _, char := range envString {
		currentChar := string(char)

		// Toggle the inside Quotes flag when encountering a double or single quote
		if currentChar == "\"" {
			insideDoubleQuotes = !insideDoubleQuotes
		} else if currentChar == "'" {
			insideSingleQuotes = !insideSingleQuotes
		}

		if !insideDoubleQuotes && !insideSingleQuotes {
			// When outside of quotes and encountering an '='
			if currentChar == "=" && parseType == "key" {
				currentSegment += currentChar
				envKey = currentSegment
				currentSegment = ""
				parseType = "value"
				// When outside of quotes and encountering an ' '
			} else if currentChar == " " && len(currentSegment) > 0 && parseType == "value" {
				envValue = currentSegment
				envSegments = append(envSegments, envKey+envValue)
				currentSegment = ""
				parseType = "key"
				envKey = ""
				envValue = ""
			} else if currentChar == " " {
				continue
			} else {
				currentSegment += currentChar
			}
		} else {
			currentSegment += currentChar
		}
	}

	if parseType == "value" {
		envValue = currentSegment
		envSegments = append(envSegments, envKey+envValue)
	}

	// Remove the outermost quotes from each environment variable's value
	for i, envSegment := range envSegments {
		parts := strings.SplitN(envSegment, "=", 2)
		key := parts[0]
		value := parts[1]

		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		envSegments[i] = key + "=" + value
	}

	return envSegments
}

func addEnvVarIfMissing(execCommEnv []string, varName string) []string {

	for _, env := range execCommEnv {
		if strings.HasPrefix(env, varName+"=") {
			return execCommEnv
		}
	}
	if value := os.Getenv(varName); value != "" {
		execCommEnv = append(execCommEnv, varName+"="+value)
	}

	return execCommEnv
}

func handleConnection(conn net.Conn, command []byte) {
	defer conn.Close()

	var uclComm uclCommand
	err := json.Unmarshal(command, &uclComm)
	if err != nil {
		return
	}

	appName := uclComm.FormatV1.AppName
	DLog.Printf("appName = %s", appName)

	commTaskCtx := ucl.NewCommTaskCtx(appName)
	defer commTaskCtx.Cancel()

	var execCommInfo ucl.ExecCommInfo
	var execComm string
	var execCommEnv []string
	var isWaitDeps bool

	switch uclComm.FormatV1.CommandType {
	case "remote_virtio_gpu", "launch_compositors":
		if uclComm.FormatV1.Sender.Command != "" {
			makeParam := makeQtCmdParams(uclComm, "")
			makeParam = makeSenderCmdParams(uclComm, makeParam)
			execComm = uclComm.FormatV1.Sender.Command + makeParam
			execCommEnv = convertEnvVars(uclComm.FormatV1.Sender.Env)
			isWaitDeps = true
		} else if uclComm.FormatV1.Receivers[0].Command != "" {
			Receiver := uclComm.FormatV1.Receivers[0]
			makeParam := makeReceiverCmdParams(uclComm, "", Receiver)
			execComm = Receiver.Command + makeParam
			execCommEnv = convertEnvVars(Receiver.Env)
			execCommEnv = addEnvVarIfMissing(execCommEnv, "XDG_RUNTIME_DIR")
			execCommEnv = addEnvVarIfMissing(execCommEnv, "WAYLAND_DISPLAY")
			isWaitDeps = false
		}

	case "transport":
		if uclComm.FormatV1.Sender.Command != "" {
			execComm = uclComm.FormatV1.Sender.Command
			execCommEnv = convertEnvVars(uclComm.FormatV1.Sender.Env)
			isWaitDeps = true
		} else if uclComm.FormatV1.Receivers[0].Command != "" {
			Receiver := uclComm.FormatV1.Receivers[0]
			execComm = Receiver.Command
			execCommEnv = convertEnvVars(Receiver.Env)
			isWaitDeps = false
		}

	case "local":
		makeParam := makeLocalCmdParams(uclComm, "")
		execComm = uclComm.FormatV1.Local.Command + makeParam
		execCommEnv = convertEnvVars(uclComm.FormatV1.Local.Env)
		isWaitDeps = true

	default:
		ELog.Printf("command_type: %s is not supported \n", uclComm.FormatV1.CommandType)
		return
	}
	execCommInfo.ExecComm = execComm
	execCommInfo.ExecCommEnv = append([]string(nil), execCommEnv...)
	execCommInfo.IsWaitDeps = isWaitDeps

	var subWg sync.WaitGroup
	waitNCountChan := make(chan []byte, 1)
	waitNKeepChan := make(chan int, 1)
	sendLcmChan := make(chan []byte, 2)

	subWg.Add(1)
	go ucl.TimingWrapper(execCommInfo, sendLcmChan, waitNCountChan, commTaskCtx, &subWg)

	subWg.Add(1)
	go ucl.NkeepWorker(sendLcmChan, waitNKeepChan, commTaskCtx, &subWg)

	/* lcm connection loop */
	rcvLcmChan := make(chan []byte, 2)
	go ucl.ConnReadLoop(conn, rcvLcmChan)

	var cp ucl.ConsistencyProtocol
LOOP:
	for {
		select {
		case sendMsg := <-sendLcmChan:
			err := ucl.ConnWriteWithSize(conn, sendMsg)
			if err != nil {
				ELog.Printf("(task=%s) ERR ConnWriteWithSize : %s\n", commTaskCtx.AppName, err)
				break LOOP
			}

		case recvMsg := <-rcvLcmChan:
			if recvMsg != nil {
				DLog.Printf("recv %s\n", recvMsg)
				json.Unmarshal(recvMsg, &cp)
				if cp.CommType == "ncount" {
					waitNCountChan <- recvMsg
				} else if cp.CommType == "nkeep" {
					waitNKeepChan <- 1
				} else {
					break LOOP
				}
			} else {
				ILog.Printf("(task=%s) Disconnected from the LCM side", commTaskCtx.AppName)
				commTaskCtx.Cancel()
				break LOOP
			}
		case <-commTaskCtx.Ctx.Done():
			break LOOP
		}
	}

	subWg.Wait()
}

func dispatchComm(conn net.Conn) error {

	commType, recvBuf, err := ucl.ReadCommand(conn)
	if err != nil {
		/* An access check may have been performed by ucl-lifecycle-manager */
		conn.Close()
		return err
	}

	switch string(commType) {
	case ucl.CMD_GetAppComm:
		DLog.Printf("getAppCmd: %s", string(recvBuf))

		appComm, err := ReadAppComm(string(recvBuf))
		if err != nil {
			ILog.Printf("(task=%s) getAppCmd: %s", recvBuf, err)
		} else {
			DLog.Printf("(task=%s) getAppCmd: %s", recvBuf, appComm)
			ucl.ConnWriteWithSize(conn, appComm)
		}

		conn.Close()

	case ucl.CMD_GetExecutableAppList:
		DLog.Printf("getExecutableAppList: %s", string(recvBuf))

		appList, err := getExecutableAppList(recvBuf)
		if err != nil {
			ILog.Printf("getExecutableAppList: %s", err)
		} else {
			ILog.Printf("getExecutableAppList: %s", appList)
			ucl.ConnWriteWithSize(conn, []byte(appList))
		}

		conn.Close()

	case ucl.CMD_DistribComm:
		go handleConnection(conn, recvBuf)

		err = ucl.SendMagicCode(conn)
		if err != nil {
			return err
		}

	default:
		ELog.Printf("commType invalid err: %s", commType)
		conn.Close()
	}

	return nil
}

func acceptLoop(listenAddr string) {

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		ELog.Printf("Listen error: %s", err)
		os.Exit(1)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			ELog.Printf("Accept error: %s", err)
			continue
		}
		DLog.Printf("socket accepted")
		dispatchComm(conn)
	}
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

	vScrnDef, err := ucl.ReadVScrnDef(vScrnDefFile)
	if err != nil {
		ELog.Println("ReadVScrnDef error : ", err)
		os.Exit(1)
	}

	listenAddr, err := ucl.GetNodeAddr(vScrnDef)
	if err != nil {
		ELog.Println("GetNodeAddr error : ", err)
		os.Exit(1)
	}

	ILog.Printf("listenAddr=%s", listenAddr)

	acceptLoop(listenAddr)
}
