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
	"os"
	"ucl-tools/internal/ucl-client/dcmapi"
)

func printUsage() {

	usage := `
Usage: 
  ucl-api-comm [OPTIONS]

Options:
  -c    Specify a DCM API command (default: none)
        get_app_list 
        get_running_app_list
        get_app_status          <appName>
        launch_compositor 
        launch_compositor_async 
        stop_compositor
        run_command             <filePath>
        run                     <appName>
        run_async               <appName>
        stop                    <appName>
        stop_all
  -h    Show this message
`
	fmt.Println(usage)
}

func main() {

	var command string
	flag.Usage = printUsage
	flag.StringVar(&command, "c", "", "Specify a dcm api command (default: none)")
	flag.Parse()

	var arg0 string
	var val int

	flagCnt := len(flag.Args())
	if flagCnt > 0 {
		arg0 = flag.Arg(0)
	}

	switch command {
	case "launch_compositor":
		val = dcmapi.DcmClientLaunchCompositor()

	case "launch_compositor_async":
		val = dcmapi.DcmClientLaunchCompositorAsync()

	case "stop_compositor":
		val = dcmapi.DcmClientStopCompositor()

	case "run_command":
		val = dcmapi.DcmClientRunAppCommand(arg0)

	case "run":
		val = dcmapi.DcmClientRunApp(arg0)

	case "run_async":
		val = dcmapi.DcmClientRunAppAsync(arg0)

	case "stop":
		val = dcmapi.DcmClientStopApp(arg0)

	case "stop_all":
		val = dcmapi.DcmClientStopAppAll()

	case "get_app_status":
		val = dcmapi.DcmClientGetAppStatus(arg0)

	case "get_app_list":
		list := dcmapi.DcmClientGetExecutableAppList()
		fmt.Fprintf(os.Stderr, "appList: %s\n", list)
		val = 0

	case "get_running_app_list":
		list := dcmapi.DcmClientGetRunningAppList()
		fmt.Fprintf(os.Stderr, "runningAppList: %s\n", list)
		val = 0

	default:
		val = -1
	}

	fmt.Fprintf(os.Stderr, "exit result %d\n", val)
}
