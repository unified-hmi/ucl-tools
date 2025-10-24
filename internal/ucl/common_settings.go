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
	"context"
	"errors"
	"net"
	"os"
	"strings"
)

const (
	STAT_ExecSuccess = "Success"
	STAT_ExecErr     = "Error"
	STAT_ExecFin     = "Finish"
	STAT_ExecBusy    = "Busy"
	STAT_AppRunning  = "AppRunning"
	STAT_AppStop     = "AppStop"
)

const (
	CMD_DistribComm              = "launchApp"
	CMD_RunApp                   = "runApp"
	CMD_RunAppAsync              = "runAppAsync"
	CMD_RunAppAsyncCb            = "runAppAsyncCb"
	CMD_StopApp                  = "stopApp"
	CMD_StopAppAll               = "stopAppAll"
	CMD_LaunchCompositors        = "launchCompositors"
	CMD_LaunchCompositorsAsync   = "launchCompositorsAsync"
	CMD_LaunchCompositorsAsyncCb = "launchCompositorsAsyncCb"
	CMD_StopCompositors          = "stopCompositors"
	CMD_GetAppStatus             = "getAppStatus"
	CMD_GetAppComm               = "getAppCmd"
	CMD_GetExecutableAppList     = "getExecutableAppList"
)

type CommTaskContext struct {
	Ctx     context.Context
	Cancel  context.CancelFunc
	AppName string
}

func NewCommTaskCtx(appName string) *CommTaskContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &CommTaskContext{
		Ctx:     ctx,
		Cancel:  cancel,
		AppName: appName,
	}
}

type MultiFlag []string

func (m *MultiFlag) String() string {
	return strings.Join(*m, ", ")
}
func (m *MultiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func GetEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetIpv4AddrsOfAllInterfaces() ([]string, error) {

	var ipaddrs []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return ipaddrs, err
	}

	/* make ipaddrs slice */
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ipv4 := ip.To4()
			if ipv4 != nil {
				ipaddrs = append(ipaddrs, ipv4.String())
			}
		}
	}

	if len(ipaddrs) == 0 {
		return ipaddrs, errors.New("Cannnot Find My IpAddr from ifaces")
	} else {
		return ipaddrs, nil
	}

}
