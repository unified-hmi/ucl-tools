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

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static inline void invokeCallback(uintptr_t f, void* arg) {
         ((void(*)(void*))f)(arg);
}
*/
import "C"
import (
	"sync"
	"ucl-tools/internal/ucl-client/dcmapi"
	"unsafe"
)

var Mutex struct {
	sync.Mutex
}

func doCallbackFunc(callBackFunc unsafe.Pointer, argPointer unsafe.Pointer) {
	Mutex.Lock()
	callback := uintptr(callBackFunc)
	if callback != 0 {
		C.invokeCallback(C.uintptr_t(callback), argPointer)
	}
	Mutex.Unlock()
}

var lastAllocated1 unsafe.Pointer
var lastAllocated2 unsafe.Pointer

//export dcm_free_executable_app_list
func dcm_free_executable_app_list() {
	Mutex.Lock()
	defer Mutex.Unlock()
	if lastAllocated1 != nil {
		C.free(lastAllocated1)
		lastAllocated1 = nil
	}
}

//export dcm_free_running_app_list
func dcm_free_running_app_list() {
	Mutex.Lock()
	defer Mutex.Unlock()
	if lastAllocated2 != nil {
		C.free(lastAllocated2)
		lastAllocated2 = nil
	}
}

func allocAndKeep(lastAllocated unsafe.Pointer, data []byte) (*C.char, C.int) {
	n := len(data)
	if n == 0 {
		return nil, 0
	}

	ptr := C.malloc(C.size_t(n + 1))
	if ptr == nil {
		return nil, 0
	}

	C.memcpy(ptr, unsafe.Pointer(&data[0]), C.size_t(n))

	endPtr := unsafe.Pointer(uintptr(ptr) + uintptr(n))
	*((*C.char)(endPtr)) = 0

	Mutex.Lock()
	lastAllocated = ptr
	Mutex.Unlock()

	return (*C.char)(ptr), C.int(n)
}

//export dcm_get_executable_app_list
func dcm_get_executable_app_list(length *C.int) *C.char {

	dcm_free_executable_app_list()

	val := dcmapi.DcmClientGetExecutableAppList()
	cstr, clen := allocAndKeep(lastAllocated1, val)
	*length = clen

	return cstr
}

//export dcm_get_running_app_list
func dcm_get_running_app_list(length *C.int) *C.char {

	dcm_free_running_app_list()

	val := dcmapi.DcmClientGetRunningAppList()
	cstr, clen := allocAndKeep(lastAllocated2, val)
	*length = clen

	return cstr
}

//export dcm_get_app_status
func dcm_get_app_status(appNameChar *C.char) C.int {
	appName := C.GoString(appNameChar)

	val := dcmapi.DcmClientGetAppStatus(appName)

	//1 = run, 0 = stop, -1 = fail
	return C.int(val)
}

//export dcm_run_app_command
func dcm_run_app_command(commandFilePathChar *C.char) C.int {
	commandFilePath := C.GoString(commandFilePathChar)

	val := dcmapi.DcmClientRunAppCommand(commandFilePath)

	//0 = end_ok, -1 = end_fail
	return C.int(val)
}

//export dcm_run_app
func dcm_run_app(appNameChar *C.char) C.int {
	appName := C.GoString(appNameChar)

	val := dcmapi.DcmClientRunApp(appName)

	//0 = end_ok, -1 = end_fail
	return C.int(val)
}

//export dcm_run_app_async
func dcm_run_app_async(appNameChar *C.char) C.int {
	appName := C.GoString(appNameChar)

	val := dcmapi.DcmClientRunAppAsync(appName)

	//1 = start, 0 = end_ok, -1 = end_fail
	return C.int(val)
}

//export dcm_run_app_async_cb
func dcm_run_app_async_cb(appNameChar *C.char, callback unsafe.Pointer, argPointer unsafe.Pointer) C.int {
	callBackFunc := (unsafe.Pointer)(callback)
	appName := C.GoString(appNameChar)

	go dcmapi.DcmClientRunAppAsyncCb(appName, doCallbackFunc, callBackFunc, argPointer)

	return 0
}

//export dcm_stop_app
func dcm_stop_app(appNameChar *C.char) C.int {
	appName := C.GoString(appNameChar)

	val := dcmapi.DcmClientStopApp(appName)

	//0 = end_ok, -1 = end_fail
	return C.int(val)
}

//export dcm_stop_app_all
func dcm_stop_app_all() C.int {

	val := dcmapi.DcmClientStopAppAll()

	//0 = end_ok, -1 = end_fail
	return C.int(val)
}

//export dcm_launch_compositor
func dcm_launch_compositor() C.int {

	val := dcmapi.DcmClientLaunchCompositor()

	//0 = end_ok, -1 = end_fail
	return C.int(val)
}

//export dcm_launch_compositor_async
func dcm_launch_compositor_async() C.int {

	val := dcmapi.DcmClientLaunchCompositorAsync()

	//1 = start, 0 = end_ok, -1 = end_fail
	return C.int(val)

}

//export dcm_launch_compositor_async_cb
func dcm_launch_compositor_async_cb(callback unsafe.Pointer, argPointer unsafe.Pointer) C.int {
	callBackFunc := (unsafe.Pointer)(callback)

	go dcmapi.DcmClientLaunchCompositorAsyncCb(doCallbackFunc, callBackFunc, argPointer)

	return 0
}

//export dcm_stop_compositor
func dcm_stop_compositor() C.int {

	val := dcmapi.DcmClientStopCompositor()

	//0 = ok, -1 = fail
	return C.int(val)
}

func main() {}
