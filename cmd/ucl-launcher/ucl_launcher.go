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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"ucl-tools/internal/ucl"
	. "ucl-tools/internal/ulog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
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
	IviSurfaceId       *int   `json:"ivi_surface_id"`
	AppId              string `json:"app_id"`
	ScanOutX           int    `json:"scanout_x"`
	ScanOutY           int    `json:"scanout_y"`
	ScanOutW           int    `json:"scanout_w"`
	ScanOutH           int    `json:"scanout_h"`
	ListenPort         int    `json:"listen_port"`
	InitialScreenColor string `json:"initial_screen_color"`
}

type iviLayerParams_t struct {
	IviLayerId    int `json:"ivi_layer_id"`
	IviSurfaceId  int `json:"ivi_surface_id"`
	OriginalSizeW int `json:"original_size_w"`
	OriginalSizeH int `json:"original_size_h"`
	SrcX          int `json:"src_x"`
	SrcY          int `json:"src_y"`
	SrcW          int `json:"src_w"`
	SrcH          int `json:"src_h"`
	DstX          int `json:"dst_x"`
	DstY          int `json:"dst_y"`
	DstW          int `json:"dst_w"`
	DstH          int `json:"dst_h"`
}

type sender_t struct {
	Launcher       ucl.UclNode      `json:"launcher"`
	Command        string           `json:"command"`
	FrontendParams frontendParams_t `json:"frontend_params"`
	Env            string           `json:"env"`
	Appli          string           `json:"appli"`
}

type receiver_t struct {
	Launcher       ucl.UclNode        `json:"launcher"`
	Command        string             `json:"command"`
	BackendParams  backendParams_t    `json:"backend_params"`
	Env            string             `json:"env"`
	IviLayerParams []iviLayerParams_t `json:"ivi_layer_params"`
}

type local_t struct {
	Launcher ucl.UclNode `json:"launcher"`
	Command  string      `json:"command"`
	Env      string      `json:"env"`
}

type uclCommand struct {
	FormatV1 struct {
		AppName string `json:"appli_name"`

		CommandType string `json:"command_type"`

		TimingControl ucl.UclNode `json:"timing_control"`

		ConsistencyKeep ucl.UclNode `json:"consistency_keep"`

		Sender sender_t `json:"sender"`

		Receivers []receiver_t `json:"receivers"`

		Local local_t `json:"local"`
	} `json:"format_v1"`
}

func execWaitApp(stdinComm string, commEnvs []string, ckeepIp string, ckeepPort int, appName string, wg *sync.WaitGroup) {

	defer wg.Done()

	var b bytes.Buffer
	b.Write([]byte(stdinComm))

	appCmd := exec.Command("ucl-consistency-keeper", ckeepIp, strconv.Itoa(ckeepPort), appName)

	appCmd.Stdin = &b
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	appCmd.Env = os.Environ()
	appCmd.Env = append(appCmd.Env, commEnvs...)
	err := appCmd.Start()
	if err != nil {
		ELog.Println("cmd.Start fail")
		return
	}
	pid := appCmd.Process.Pid
	DLog.Printf("wait process(app=%s pid:%d)\n", appName, pid)
	appCmd.Wait()
	ILog.Printf("finish process(app=%s pid:%d)\n", appName, pid)
}

func isJSON(b []byte) bool {
	var js map[string]interface{}
	return json.Unmarshal(b, &js) == nil
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

	if rparam.AppId != "" {
		makeParam = makeParam + " -a " + rparam.AppId
	}

	return makeParam
}

func createTimingWrapperComm(execComm string, timingIp string, timingPort int, appName string, sync bool) string {
	syncOpt := " "
	if sync {
		syncOpt = " -p "
	}
	newExecComm := "ucl-timing-wrapper" + syncOpt + timingIp + " " + strconv.Itoa(timingPort) + " " + appName + " " + execComm
	return newExecComm
}

func convertEnvVars(envString string) []string {
	var envSegments []string
	insideSingleQuotes := false
	insideDoubleQuotes := false
	parseType := "key"
	envKey := ""
	envValue := ""

	currentSegment := ""
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

func handleConnection(uclComm uclCommand, listenIp string, listenPort int) {

	appName := uclComm.FormatV1.AppName
	ILog.Printf("appName=%s\n", appName)

	ckeepIp := uclComm.FormatV1.ConsistencyKeep.Ip
	ckeepPort := uclComm.FormatV1.ConsistencyKeep.Port
	DLog.Println("ckeep:", ckeepIp, ":", ckeepPort)

	timingIp := uclComm.FormatV1.TimingControl.Ip
	timingPort := uclComm.FormatV1.TimingControl.Port
	DLog.Println("timing:", timingIp, timingPort)

	var wg sync.WaitGroup
	var commArray []string
	commEnvMap := make(map[string][]string)
	makeParam := ""
	switch uclComm.FormatV1.CommandType {
	case "remote_virtio_gpu":
		if node := uclComm.FormatV1.Sender.Launcher; node.Ip == listenIp && node.Port == listenPort {
			ILog.Println("I'm sender")
			makeParam = makeSenderCmdParams(uclComm, makeParam)
			execComm := uclComm.FormatV1.Sender.Command + makeParam
			twExecComm := createTimingWrapperComm(execComm, timingIp, timingPort, appName, false)
			commArray = append(commArray, twExecComm)
			if len(uclComm.FormatV1.Sender.Env) > 0 {
				execCommEnv := convertEnvVars(uclComm.FormatV1.Sender.Env)
				commEnvMap[twExecComm] = execCommEnv
			}
		}

		for _, r := range uclComm.FormatV1.Receivers {
			if node := r.Launcher; node.Ip == listenIp && node.Port == listenPort {
				ILog.Println("I'm receiver")
				makeParam = makeReceiverCmdParams(uclComm, "", r)
				execComm := r.Command + makeParam
				twExecComm := createTimingWrapperComm(execComm, timingIp, timingPort, appName, true)
				commArray = append(commArray, twExecComm)
				if len(r.Env) > 0 {
					execCommEnv := convertEnvVars(r.Env)
					commEnvMap[twExecComm] = execCommEnv
				}
			}
		}

	case "transport":
		if node := uclComm.FormatV1.Sender.Launcher; node.Ip == listenIp && node.Port == listenPort {
			ILog.Println("I'm sender")
			execComm := uclComm.FormatV1.Sender.Command
			twExecComm := createTimingWrapperComm(execComm, timingIp, timingPort, appName, false)
			commArray = append(commArray, twExecComm)
			if len(uclComm.FormatV1.Sender.Env) > 0 {
				execCommEnv := convertEnvVars(uclComm.FormatV1.Sender.Env)
				commEnvMap[twExecComm] = execCommEnv
			}
		}

		for _, r := range uclComm.FormatV1.Receivers {
			if node := r.Launcher; node.Ip == listenIp && node.Port == listenPort {
				ILog.Println("I'm receiver")
				execComm := r.Command
				twExecComm := createTimingWrapperComm(execComm, timingIp, timingPort, appName, false)
				commArray = append(commArray, twExecComm)
				if len(r.Env) > 0 {
					execCommEnv := convertEnvVars(r.Env)
					commEnvMap[twExecComm] = execCommEnv
				}
			}
		}

	case "local":
		execComm := uclComm.FormatV1.Local.Command
		twExecComm := createTimingWrapperComm(execComm, timingIp, timingPort, appName, false)
		commArray = append(commArray, twExecComm)
		if len(uclComm.FormatV1.Local.Env) > 0 {
			execCommEnv := convertEnvVars(uclComm.FormatV1.Local.Env)
			commEnvMap[twExecComm] = execCommEnv
		}

	default:
		ELog.Printf("command_type: %s is not supported \n", uclComm.FormatV1.CommandType)
		os.Exit(1)
	}

	for _, r := range commArray {
		ILog.Println("[new exec comm] : ", r)
		wg.Add(1)
		go execWaitApp(r, commEnvMap[r], ckeepIp, ckeepPort, appName, &wg)
	}

	wg.Wait()
}

func dispatchComm(conn net.Conn, listenIp string, listenPort int) {
	defer conn.Close()

	var (
		err error
	)

	cbio := bufio.NewReader(conn)

	recvComm, err := ucl.ConnReadWithSize(cbio)
	if err != nil {
		DLog.Println("ConnReadWithSize error: ", err)
		return
	}
	DLog.Println("receive command type: ", string(recvComm))

	recvBuf, err := ucl.ConnReadWithSize(cbio)
	if err != nil {
		DLog.Println("ConnReadWithSize error: ", err)
		return
	}
	DLog.Println("receive message: ", string(recvBuf))

	switch string(recvComm) {
	case "launchApp":
		if !isJSON(recvBuf) {
			ELog.Println("invalid json")
			return
		}

		var uclComm uclCommand
		err = json.Unmarshal(recvBuf, &uclComm)
		if err != nil {
			ELog.Println("json decode error: ", err)
			return
		}

		go handleConnection(uclComm, listenIp, listenPort)

	case "checkSocket":
		ip, port, err := net.SplitHostPort(string(recvBuf))
		if err == nil {
			targetAddr := ip + ":" + port
			chk := false
			for i := 0; i < 10; i++ {
				chkConn, err := net.Dial("tcp", targetAddr)
				if err == nil {
					chk = true
					chkConn.Close()
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if chk == false {
				DLog.Printf("connect to %s: error: %s", targetAddr, err)
				return
			}
		} else {
			DLog.Printf("SplitHostPort error: %s", err)
			return
		}
	}

	magicBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBuf, ucl.MagicCode)
	ucl.ConnWrite(conn, magicBuf)
}

func acceptLoop(listenIp string, listenPort int) {

	listenAddr := listenIp + ":" + strconv.Itoa(listenPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		ELog.Printf("Listen error: %s\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		recvBuf, err := ucl.ConnRead(conn, 4)
		if err == nil {
			recvCode := binary.BigEndian.Uint32(recvBuf)
			if recvCode == ucl.MagicCode {
				dispatchComm(conn, listenIp, listenPort)
			} else {
				DLog.Println("magicCode mismatch!")
				conn.Close()
			}
		} else {
			conn.Close()
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "%s [option] | listenIp ListenPort\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "[option]\n")
	flag.PrintDefaults()
}

func main() {

	flag.Usage = printUsage

	var (
		verbose      bool
		debug        bool
		vScrnDefFile string
		keyNodeId    int
		keyHostName  string
	)

	flag.BoolVar(&verbose, "v", true, "verbose info log")
	flag.BoolVar(&debug, "d", false, "verbose debug log")
	flag.StringVar(&vScrnDefFile, "f", ucl.VSCRNDEF_FILE, "virtual-screen-def.json file Path")

	flag.IntVar(&keyNodeId, "N", -1, "search ucl-launchar param by node_id from VScrnDef file")
	flag.StringVar(&keyHostName, "H", "", "search ucl-launchar param by hostname from VScrnDef file")

	flag.Parse()

	if verbose == true {
		ILog.SetOutput(os.Stderr)
	}

	if debug == true {
		DLog.SetOutput(os.Stderr)
	}

	DLog.Printf("ARG0:%s, ARG1:%s\n", flag.Arg(0), flag.Arg(1))

	var (
		err        error
		listenIp   string
		listenPort int
		nodeId     int
		vscrnDef   *ucl.VScrnDef
	)

	flagCnt := len(flag.Args())
	switch flagCnt {
	case 0:

		vscrnDef, err = ucl.ReadVScrnDef(vScrnDefFile)
		if err != nil {
			ELog.Println("ReadVScrnDef error : ", err)
			return
		}

		if keyNodeId != -1 && keyHostName != "" {
			ELog.Println("error multiple search keys")
			return
		}

		if keyNodeId != -1 {
			nodeId = keyNodeId

			listenIp, err = ucl.GetIpAddr(nodeId, vscrnDef)
			if err != nil {
				ELog.Println("GetIpAddr error : ", err)
				return
			}
		} else {
			if keyHostName == "" {
				keyHostName, err = os.Hostname()
				if err != nil {
					ELog.Println("os.Hostname fail", err)
					return
				}
			}
			ipAddrs, err := ucl.GetIpAddrsOfAllInterfaces()
			if err != nil {
				ELog.Println("GetIpAddrsOfAllInterfaces error : ", err)
				return
			}
			nodeId, listenIp, err = ucl.GetNodeIdAndIpAddr(ipAddrs, keyHostName, vscrnDef)
			if err != nil {
				ELog.Println("GetNodeIdAndIpAddr error : ", err)
				return
			}
		}

		listenPort, err = ucl.GetPort(nodeId, vscrnDef)
		if err != nil {
			ELog.Println("GetPort error : ", err)
			return
		}

	case 2:
		listenIp = flag.Arg(0)
		listenPort, _ = strconv.Atoi(flag.Arg(1))

	default:
		printUsage()
	}

	ILog.Printf("(listenIp, listenPort)=(%s, %d)\n", listenIp, listenPort)

	acceptLoop(listenIp, listenPort)
}
