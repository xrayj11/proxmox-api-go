package commands

import (
	_ "github.com/xrayj11/proxmox-api-go/cli/command/content"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/content/iso"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/content/template"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/create"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/create/guest"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/delete"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/example"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/get"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/get/guest"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/get/id"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/guest"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/guest/qemu"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/list"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/member"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/member/group"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/node"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/set"
	_ "github.com/xrayj11/proxmox-api-go/cli/command/update"
)
