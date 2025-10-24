# Unified Clustering Tools (UCL)

> Unified Clustering Tools (UCL) is a framework for ensuring the consistency of the cluster application.
> This framework launch applications on multiple Socs/VMs/PCs. It ensures the startup order of related processes, monitors their operational status, and reliably reclaims resources when they are no longer needed.
> UCL is divided into Master and Worker. 
> Worker runs constantly on each host and waits for the connection from Master and Master Communicates application execution information to Worker.

## Contents

- [Unified Clustering Tools (UCL)](#architecture)
  - [Contents](#contents)
  - [Repository structure](#repository-structure)
  - [How to install](#how-to-install)
    - [Golang setup](#golang-setup)
    - [Build UCL framework](#build-ucl-framework)
  - [How to Use](#how-to-use)
    - [Json settings](#json-settings)
    - [Workers side](#workers-side)
    - [Manager side](#manager-side)
    - [Command request](#command-request)
  - [Collaborate with RVGPU](#collaborate-with-rvgpu)
    - [How to install RVGPU](#how-to-install-rvgpu)
    - [How to install rvgpu-wlproxy](#how-to-install-rvgpu-wlproxy)
    - [How to run UCL with RVGPU v1.1.0 or higher](#how-to-run-ucl-with-rvgpu-v1-or-higher)
      - [Run ucl-virtio-gpu-wl-recv](#run-ucl-virtio-gpu-wl-recv)
      - [Run ucl-virtio-gpu-wl-send](#run-ucl-virtio-gpu-wl-send)
      - [Using Json configuration](#using-json-configuration-v1)
        - [Json settings](#json-settings-v1)
        - [Workers side](#workers-side-v1)
        - [Manager side](#manager-side-v1)
        - [Command request](#command-request-v1)
    - [How to run UCL with RVGPU v2.0.0 or higher](#how-to-run-ucl-with-rvgpu-v2-or-higher)
      - [Using Json configuration](#using-json-configuration-v2)
        - [Json settings](#json-settings-v2)
        - [Workers side](#workers-side-v2)
        - [Manager side](#manager-side-v2)
        - [Command request](#command-request-v2)

## Repository structure

```
.
├── cmd
│   ├── Makefile
│   ├── ucl-api-comm
│   │   ├── Makefile
│   │   └── ucl_api_comm.go
│   ├── ucl-lifecycle-manager
│   │   ├── Makefile
│   │   └── ucl_lifecycle_manager.go
│   ├── ucl-node
│   │   ├── Makefile
│   │   └── ucl_node.go
│   ├── ucl-virtio-gpu-rvgpu-compositor
│   │   ├── Makefile
│   │   └── ucl_virtio_gpu_rvgpu_compositor.go
│   ├── ucl-virtio-gpu-wl-recv
│   │   ├── Makefile
│   │   └── ucl_virtio_gpu_wl_recv.go
│   └── ucl-virtio-gpu-wl-send
│       ├── Makefile
│       └── ucl_virtio_gpu_wl_send.go
├── CONTRIBUTING.md
├── example
│   ├── rvgpu-1.2.0
│   │   ├── multi-node
│   │   │   ├── glmark2-es2-wayland
│   │   │   │   └── app.json
│   │   │   └── virtual-screen-def.json
│   │   └── single-node
│   │       ├── glmark2-es2-wayland
│   │       │   └── app.json
│   │       └── virtual-screen-def.json
│   └── rvgpu-2.0.0
│       ├── multi-node
│       │   ├── glmark2-es2-wayland
│       │   │   └── app.json
│       │   └── virtual-screen-def.json
│       └── single-node
│           ├── glmark2-es2-wayland
│           │   └── app.json
│           └── virtual-screen-def.json
├── internal
│   ├── Makefile
│   ├── ucl
│   │   ├── common_settings.go
│   │   ├── ucl_consistency_keeper.go
│   │   ├── ucl_node_conn.go
│   │   ├── ucl_proc_sighandler.go
│   │   └── virtual_screen_def.go
│   ├── ucl-client
│   │   ├── dcmapi
│   │   │   ├── dcmapi_protocol.go
│   │   │   ├── dcm_client_api.go
│   │   │   └── Makefile
│   │   └── Makefile
│   └── ulog
│       ├── Makefile
│       └── ulog.go
├── LICENSE.md
├── Makefile
├── pkg
│   └── ucl-client-lib
│       ├── Makefile
│       └── ucl_client.go
├── proto
│   ├── dcm.proto
│   └── grpc
│       └── dcm
│           ├── dcm_grpc.pb.go
│           └── dcm.pb.go
└── README.md
```


# How to install
## Golang setup
Before building UCL API, you need to install and configure Golang.
```
sudo apt-get install golang
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
```

## Build UCL framework
You can easily build UCL framework using make.
```
mkdir -p $GOPATH/src
cd $GOPATH/src
git clone https://github.com/unified-hmi/ucl-tools.git
cd ucl-tools
make
```

# How to use
UCL can launch applications on multiple SoCs/VMs/PCs, control the launch timing and check for survival.  
UCL API is implemented by using gRPC protocol. So, you can implement UCL Client APIs within your applications by importing gRPC protocol header files.
When a UCL Client API is called, the gRPC server (ucl-lifecycle-manager) controls multiple workers(ucl-node) and launches the appropriate application based on an command request.

## <a name="json-settings"></a>Json settings
To run UCL, two Json files are required.
* virtual-screen-def.json: execution environment such as display and node(SoCs/VMs/PCs) informations.
* app.json: application execution information such as target node and application execution command.

Json files must be created correctly for your execution environment.

## <a name="workers-side"></a>Workers side
Before running command request, the worker side needs to launch __*ucl-node*__.  
__*ucl-node*__ needs the path to virtual-screen-def.json, so please set it with "-f" option. (default path is /etc/uhmi-framework/virtual-screen-def.json)

**Note:** Master node may also work as worker. 

- Options of ucl-node
  - -d: verbose debug log
  - -f: virtual-screen-def.json file Path (default "/etc/uhmi-framework/virtual-screen-def.json")
  - -v: verbose info log (default true)

```
ucl-node -f <path to virtual-screen-def.json>
```

## <a name="manager-side"></a>Manager side
Before running command request, the manager side needs to launch __*ucl-lifecycle-manager*__.  
It works as a gRPC Server and get requests from gRPC Client APIs.

__*ucl-lifecycle-manager*__ needs the path to virtual-screen-def.json, so please set it with "-f" option. (default path is /etc/uhmi-framework/virtual-screen-def.json)

- Options of ucl-lifecycle-manager
  - -d: verbose debug log
  - -f: virtual-screen-def.json file Path (default "/etc/uhmi-framework/virtual-screen-def.json")
  - -v: verbose info log (default true)

```
ucl-lifecycle-manager -f <path to virtual-screen-def.json>
```


## <a name="command-request"></a>Command request
After launch manager and all workers.  
__*ucl-api-comm*__ can send control commands to the manager as gRPC Client API.

- Options of ucl-api-comm
  - -c: Specify a DCM API command (default: none)  
       `get_app_list`
       `get_running_app_list`
       `get_app_status          <appName>`
       `launch_compositor`
       `launch_compositor_async`
       `stop_compositor`
       `run_command             <filePath>`
       `run                     <appName>`
       `run_async               <appName>`
       `stop                    <appName>`
       `stop_all`
  - -h: Show this message


```
ucl-api-comm -c <command>
```


# Collaborate with RVGPU
UCL can be combined with RVGPU in our repository to display applications remotly.  
rvgpu-wlproxy enables to remotely display wayland client applications using RVGPU.  
UCL has __*ucl-virtio-gpu-wl-send/recv*__, which can easily execute and effectively utilize RVGPU and rvgpu-wlproxy.  

## How to install RVGPU
For instructions on how to install remove-virtio-gpu, please refer to the [README](https://github.com/unified-hmi/remote-virtio-gpu).

## How to install rvgpu-wlproxy
For instructions on how to install rvgpu-wlproxy, please refer to the [README](https://github.com/unified-hmi/rvgpu-wlproxy).

## <a name="how-to-run-ucl-with-rvgpu-v1-or-higher"></a>How to run UCL with RVGPU v1.1.0 or higher
By launching __*ucl-virtio-gpu-wl-send*__ on RVGPU client side and __*ucl-virtio-gpu-wl-recv*__ on RVGPU server side, it can be executed without detailed configuration.  
Additionally, by using the app.json, remote rendering of the application across multiple nodes can be achieved easily.

**Note:** __*ucl-virtio-gpu-wl-send/recv*__ only support RVGPU v1.1.0 or higher

### Run ucl-virtio-gpu-wl-recv
__*ucl-virtio-gpu-wl-recv*__  optionally receives information about RVGPU server and executes the rvgpu-renderer.

- Options
  - -B: remote-virtio-gpu reciever color config (default "0x33333333")
  - -P: specify remote-virtio-gpu reciever port (default "55667")
  - -S: specify remote-virtio-gpu reciever surface ID (default "9000")
  - -d: verbose debug log
  - -s: remote-virtio-gpu reciever screen config (default "1920x1080@0,0")

```
ucl-virtio-gpu-wl-recv -s 1920x1080@0,0 -P 55667
```

### Run ucl-virtio-gpu-wl-send
__*ucl-virtio-gpu-wl-send*__ optionally receives information about RVGPU client and executes rvgpu-proxy and rvgpu-wlproxy to render Wayland client applications remotely.

- Options
  - -appli_name: specify application name (default "ucl-virtio-gpu-wl-send")
  - -d: verbose debug log
  - -n: Specify multiple -n options (default 127.0.0.1:55667)
  - -s: remote-virtio-gpu sender screen config (default "1920x1080@0,0")
  - -v: verbose info log (default true)

```
sudo -i
modprobe virtio-gpu
modprobe virtio-lo
ucl-virtio-gpu-wl-send -s 1920x1080@0,0 -n 127.0.0.1:55667 glmark2-es2-wayland
```

### <a name="using-json-configuration-v1"></a>Using Json configuration
To launch RVGPU and rvgpu-wlproxy from UCL, select __*ucl-virtio-gpu-wl-send/recv*__ as the application to be launched by UCL in app.json.

#### <a name="json-settings-v1"></a>Json settings
Sample Json files are located in the "$GOPATH/src/ucl-tools/example/rvgpu-1.2.0" directory.
Please modify Json files according to your own execution environment referring to samples.

#### <a name="workers-side-v1"></a>Workers side
Execute the UCL command for Worker on the all hosts on which you want to run the application.
```
sudo -i
modprobe virtio-gpu
modprobe virtio-lo
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
export DCMPATH=$GOPATH/src/ucl-tools/example/rvgpu-1.2.0/single-node
ucl-node -f $GOPATH/src/ucl-tools/example/rvgpu-1.2.0/single-node/virtual-screen-def.json
```

#### <a name="manager-side-v1"></a>Manager side
After all Worker commands have been executed, launch Manager command on any host.
```
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
ucl-lifecycle-manager -f $GOPATH/src/ucl-tools/example/rvgpu-1.2.0/single-node/virtual-screen-def.json
```

#### <a name="command-request-v1"></a>Command request
After all Worker and Manager commands have been executed, launch command request on any host.
```
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
ucl-api-comm -c run glmark2-es2-wayland
```

## <a name="how-to-run-ucl-with-rvgpu-v2-or-higher"></a>How to run UCL with RVGPU v2.0.0 or higher
Using app.json and virtual-screen-def.json, you can easily achieve the functionality of displaying multiple rvgpu-proxy renderings with a single rvgpu-renderer.  
This feature is supported in RVGPU 2.0.0.

### <a name="using-json-configuration-v2"></a>Using Json configuration
To launch RVGPU and rvgpu-wlproxy from UCL, select __*ucl-virtio-gpu-wl-send*__ as the application to be launched by UCL in app.json.  
rvgpu-renderer information must be written in virtual-screen-def.json to the following as an example:
```
"framework_node": [
    {"node_id": 0,"ula": {"debug": false, "debug_port": 8080, "port": 10100},"ucl_node": {"port": 7654},
     "compositor":[{"vdisplay_ids":[0], "sock_domain_name": "rvgpu-compositor-0", "listen_port":36000}]
    }
]
```


#### <a name="json-settings-v2"></a>Json settings
Sample Json files are located in the "$GOPATH/src/ucl-tools/example/rvgpu-2.0.0" directory.
Please modify Json files according to your own execution environment referring to samples.

#### <a name="workers-side-v2"></a>Workers side
Execute the UCL command for Worker on the all hosts on which you want to run the application.
```
sudo -i
modprobe virtio-gpu
modprobe virtio-lo
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
export DCMPATH=$GOPATH/src/ucl-tools/example/rvgpu-2.0.0/single-node
ucl-node -f $GOPATH/src/ucl-tools/example/rvgpu-2.0.0/single-node/virtual-screen-def.json
```
**Note:** The default path is as follows:  
  - DCMPATH: "/var/locao/uhmi-app"  
  
#### <a name="manager-side-v2"></a>Manager side
After all Worker commands have been executed, launch Manager command on any host.
```
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
export RVGPU_LAUNCH_COMM_PATH=$GOBIN
ucl-lifecycle-manager -f $GOPATH/src/ucl-tools/example/rvgpu-2.0.0/single-node/virtual-screen-def.json
```
**Note:** The default path is as follows:  
  - RVGPU_LAUNCH_COMM_PATH: "/usr/bin"  


#### <a name="command-request-v2"></a>Command request
After all Worker and Manager commands have been executed, launch command request on any host.
```
export GOPATH=<your go work directory>
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
ucl-api-comm -c launch_compositor_async
ucl-api-comm -c run glmark2-es2-wayland
```


#### Run dcm API
UCL also provides a C language shared library (default: generated in $GOPATH/pkg/libuclclient).
This library makes it easy to issue command requests to the manager.

- Description of dcm API
  - dcm_launch_compositor: launch rvgpu-renderer based on the compositor section of virtual-screen-def.json.
  - dcm_launch_compositor_async: execute dcm_launch_compositor asynchronously.
  - dcm_stop_compositor: stop the compositor launched by dcm_launch_compositor.
  - dcm_run_app: find the app.json of the specified app_name in the system and control the worker by taking over the app.json.
  - dcm_run_app_async: execute dcm_run_app asynchronously.
  - dcm_run_app_command: receive app.json as an argument and control the worker by taking over the app.json.
  - dcm_stop_app: stop the worker task launched by dcm_run_app.
  - dcm_stop_app_all: stop the all worker task launched by dcm_run_app.
  - dcm_get_app_status: check the status of the task for the specified app_name.
  - dcm_get_executable_app_list: Get a list of app names that have executable app.json.
  - dcm_get_running_app_list: Get a list of running app names.

Please create a C language source code with content similar to the following as an example:
```c
#include <stdio.h>
#include "libuclclient.h"

int main(void)  
{
	dcm_run_app("glmark2-es2-wayland");

	return 0;
}
```

Compile this source code with gcc as follows:
```
gcc -I./ -L./ sample.c -luclclient -o sample.out
```

Execute the dcm API with a command as follows:
```
export VSDPATH="<path to virtual-screen-def.json>"
./sample.out
```
