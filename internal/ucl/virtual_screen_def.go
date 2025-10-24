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
	"io/ioutil"
	"os"
	"strconv"
)

const VSCRNDEF_FILE = "/etc/uhmi-framework/virtual-screen-def.json"

type VScrnDef struct {
	RealDisplays []struct {
		NodeId     int `json:"node_id"`
		VDisplayId int `json:"vdisplay_id"`
		PixelW     int `json:"pixel_w"`
		PixelH     int `json:"pixel_h"`
		RDisplayId int `json:"rdisplay_id"`
	} `json:"real_displays"`

	Nodes []struct {
		NodeId   int    `json:"node_id"`
		HostName string `json:"hostname"`
		Ip       string `json:"ip"`
	} `json:"node"`

	DistributedWindowSystem struct {
		LifeCycleManager struct {
			NodeId int `json:"node_id"`
			Port   int `json:"port"`
		} `json:"ucl_lifecycle_manager"`
		FrameworkNode []struct {
			NodeId int `json:"node_id"`
			Ucl    struct {
				Port int `json:"port"`
			} `json:"ucl_node"`
			Compositor []struct {
				VDisplayIds    []int  `json:"vdisplay_ids"`
				IviSurfaceId   int    `json:"ivi_surface_id"`
				SockDomainName string `json:"sock_domain_name"`
				ListenPort     int    `json:"listen_port"`
			} `json:"compositor"`
		} `json:"framework_node"`
	} `json:"distributed_window_system"`
}

type UclNode struct {
	Ip       string `json:"ip"`
	Port     int    `json:"port"`
	HostName string `json:"hostname"`
}

func ReadVScrnDef(vsdPath ...string) (*VScrnDef, error) {
	var fname string
	if len(vsdPath) > 0 && vsdPath[0] != "" {
		fname = vsdPath[0]
	} else {
		fname = GetEnv("VSDPATH", VSCRNDEF_FILE)
	}

	f, err := os.Open(fname)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("%s %s", fname, err))
	}
	defer f.Close()

	jsonBytes, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("ReadAll error: ", err))
	}

	var vscrnDef VScrnDef
	err = json.Unmarshal(jsonBytes, &vscrnDef)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("json Unmarshal error: %s", err))
	}

	return &vscrnDef, nil
}

func (vdef *VScrnDef) GetNodeIdByHostName(hostname string) (int, error) {

	for _, r := range vdef.Nodes {
		if hostname == r.HostName {
			return r.NodeId, nil
		}
	}

	return -1, errors.New("Cannot Find My NodeId from VScrnDef json")
}

func (vdef *VScrnDef) GetIpAddrByNodeIdAndIpCandidateList(ipAddrs []string, nodeId int) (string, error) {

	for _, r := range vdef.Nodes {
		if nodeId == r.NodeId {
			for _, ipaddr := range ipAddrs {
				if ipaddr == r.Ip {
					return ipaddr, nil
				}
			}
		}
	}

	return "0.0.0.0", errors.New("The acquired IP address does not exist in VScrnDef json")
}

func GetMyNodeId(vscrnDef *VScrnDef) (int, error) {
	keyHostName, err := os.Hostname()
	if err != nil {
		return -1, err
	}
	nodeId, err := vscrnDef.GetNodeIdByHostName(keyHostName)
	if err != nil {
		return -1, err
	}
	return nodeId, nil
}

func GetMyIpv4Addr(vscrnDef *VScrnDef) (string, error) {

	keyHostName, err := os.Hostname()
	if err != nil {
		return "", err
	}

	nodeId, err := vscrnDef.GetNodeIdByHostName(keyHostName)
	if err != nil {
		return "", err
	}
	ipAddrs, err := GetIpv4AddrsOfAllInterfaces()
	if err != nil {
		return "", err
	}
	ipAddr, err := vscrnDef.GetIpAddrByNodeIdAndIpCandidateList(ipAddrs, nodeId)
	if err != nil {
		return "", err
	}
	return ipAddr, nil
}

func GetIpAddr(nodeId int, vscrnDef *VScrnDef) (string, error) {

	for _, r := range vscrnDef.Nodes {
		if nodeId == r.NodeId {
			return r.Ip, nil
		}
	}

	return "0.0.0.0", errors.New("Connot Find My IP Address from VScrnDef json")
}

func GetNodeIdAndIpAddr(ipAddrs []string, hostname string, vscrnDef *VScrnDef) (int, string, error) {

	for _, r := range vscrnDef.Nodes {
		if hostname == r.HostName {
			for _, ipaddr := range ipAddrs {
				if ipaddr == r.Ip {
					return r.NodeId, r.Ip, nil
				}
			}
		}
	}

	return -1, "0.0.0.0", errors.New("Cannot Find My NodeId and IP Address from VScrnDef json")
}

func GetNodePort(nodeId int, vscrnDef *VScrnDef) (int, error) {

	for _, r := range vscrnDef.DistributedWindowSystem.FrameworkNode {
		if nodeId == r.NodeId {
			return r.Ucl.Port, nil
		}
	}

	return -1, errors.New("Cannnot Find My Port from VScrnDef json")
}

func GetNodeAddr(vscrnDef *VScrnDef) (string, error) {

	keyHostName, err := os.Hostname()
	if err != nil {
		return "", err
	}

	ipAddrs, err := GetIpv4AddrsOfAllInterfaces()
	if err != nil {
		return "", err
	}

	nodeId, targetIp, err := GetNodeIdAndIpAddr(ipAddrs, keyHostName, vscrnDef)
	if err != nil {
		return "", err
	}

	targetPort, err := GetNodePort(nodeId, vscrnDef)
	if err != nil {
		return "", err
	}

	targetAddr := targetIp + ":" + strconv.Itoa(targetPort)

	return targetAddr, nil

}

func GetLcmPort(nodeId int, vscrnDef *VScrnDef) (int, error) {

	if nodeId == vscrnDef.DistributedWindowSystem.LifeCycleManager.NodeId {
		return vscrnDef.DistributedWindowSystem.LifeCycleManager.Port, nil
	}

	return -1, errors.New("Cannnot Find My Port from VScrnDef json")
}

func GetLcmNodeAddr(vscrnDef *VScrnDef) (string, error) {

	var targetAddr string

	for _, node := range vscrnDef.Nodes {
		if node.NodeId == vscrnDef.DistributedWindowSystem.LifeCycleManager.NodeId {
			port := vscrnDef.DistributedWindowSystem.LifeCycleManager.Port
			ipAddr := node.Ip
			targetAddr = ipAddr + ":" + strconv.Itoa(port)
			return targetAddr, nil
		}
	}

	return targetAddr, errors.New("LifeCycleManager Node not found.")
}

func IsRecvCompositor(listenPort int, vscrnDef *VScrnDef) bool {

	for _, fwn := range vscrnDef.DistributedWindowSystem.FrameworkNode {
		for _, com := range fwn.Compositor {
			if listenPort == com.ListenPort {
				return true
			}
		}
	}
	return false

}

func GetFrameworkNode(vscrnDef *VScrnDef) (dNodes []UclNode) {

	for _, r1 := range vscrnDef.Nodes {
		var uclNode UclNode
		uclNode.Ip = r1.Ip
		uclNode.HostName = r1.HostName
		for _, r2 := range vscrnDef.DistributedWindowSystem.FrameworkNode {
			if r1.NodeId == r2.NodeId {
				uclNode.Port = r2.Ucl.Port
				dNodes = append(dNodes, uclNode)
				break
			}
		}
	}

	return
}

func ReadAliasIp(vscrnDef *VScrnDef) (aip map[string]UclNode) {

	aip = make(map[string]UclNode)
	for _, r1 := range vscrnDef.Nodes {
		var uclNode UclNode
		uclNode.Ip = r1.Ip
		for _, r2 := range vscrnDef.DistributedWindowSystem.FrameworkNode {
			if r1.NodeId == r2.NodeId {
				uclNode.Port = r2.Ucl.Port
				break
			}
		}
		aip[r1.HostName] = uclNode
	}

	return
}
