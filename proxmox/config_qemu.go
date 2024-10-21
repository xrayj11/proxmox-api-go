package proxmox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xrayj11/proxmox-api-go/internal/util"
)

// Currently ZFS local, LVM, Ceph RBD, CephFS, Directory and virtio-scsi-pci are considered.
// Other formats are not verified, but could be added if they're needed.
// const rxStorageTypes = `(zfspool|lvm|rbd|cephfs|dir|virtio-scsi-pci)`
const machineModels = `(pc|q35|pc-i440fx)`

type (
	QemuDevices     map[int]map[string]interface{}
	QemuDevice      map[string]interface{}
	QemuDeviceParam []string
	IpconfigMap     map[int]interface{}
)

// ConfigQemu - Proxmox API QEMU options
type ConfigQemu struct {
	Agent           *QemuGuestAgent  `json:"agent,omitempty"`
	Args            string           `json:"args,omitempty"`
	Bios            string           `json:"bios,omitempty"`
	Boot            string           `json:"boot,omitempty"`     // TODO should be an array of custom enums
	BootDisk        string           `json:"bootdisk,omitempty"` // TODO discuss deprecation? Only returned as it's deprecated in the proxmox api
	CPU             *QemuCPU         `json:"cpu,omitempty"`
	CloudInit       *CloudInit       `json:"cloudinit,omitempty"`
	Description     *string          `json:"description,omitempty"`
	Disks           *QemuStorages    `json:"disks,omitempty"`
	EFIDisk         QemuDevice       `json:"efidisk,omitempty"`   // TODO should be a struct
	FullClone       *int             `json:"fullclone,omitempty"` // TODO should probably be a bool
	HaGroup         string           `json:"hagroup,omitempty"`
	HaState         string           `json:"hastate,omitempty"` // TODO should be custom type with enum
	Hookscript      string           `json:"hookscript,omitempty"`
	Hotplug         string           `json:"hotplug,omitempty"`   // TODO should be a struct
	Iso             *IsoFile         `json:"iso,omitempty"`       // Same as Disks.Ide.Disk_2.CdRom.Iso
	LinkedVmId      uint             `json:"linked_id,omitempty"` // Only returned setting it has no effect
	Machine         string           `json:"machine,omitempty"`   // TODO should be custom type with enum
	Memory          *QemuMemory      `json:"memory,omitempty"`
	Name            string           `json:"name,omitempty"` // TODO should be custom type as there are character and length limitations
	Node            string           `json:"node,omitempty"` // Only returned setting it has no effect, set node in the VmRef instead
	Onboot          *bool            `json:"onboot,omitempty"`
	Pool            *PoolName        `json:"pool,omitempty"`
	Protection      *bool            `json:"protection,omitempty"`
	QemuDisks       QemuDevices      `json:"disk,omitempty"`    // DEPRECATED use Disks *QemuStorages instead
	QemuIso         string           `json:"qemuiso,omitempty"` // DEPRECATED use Iso *IsoFile instead
	QemuKVM         *bool            `json:"kvm,omitempty"`
	QemuNetworks    QemuDevices      `json:"network,omitempty"` // TODO should be a struct
	QemuOs          string           `json:"ostype,omitempty"`
	QemuPCIDevices  QemuDevices      `json:"hostpci,omitempty"` // TODO should be a struct
	QemuPxe         bool             `json:"pxe,omitempty"`
	QemuUnusedDisks QemuDevices      `json:"unused,omitempty"` // TODO should be a struct
	QemuUsbs        QemuDevices      `json:"usb,omitempty"`    // TODO should be a struct
	QemuVga         QemuDevice       `json:"vga,omitempty"`    // TODO should be a struct
	RNGDrive        QemuDevice       `json:"rng0,omitempty"`   // TODO should be a struct
	Scsihw          string           `json:"scsihw,omitempty"` // TODO should be custom type with enum
	Serials         SerialInterfaces `json:"serials,omitempty"`
	Smbios1         string           `json:"smbios1,omitempty"` // TODO should be custom type with enum?
	Startup         string           `json:"startup,omitempty"` // TODO should be a struct?
	Storage         string           `json:"storage,omitempty"` // this value is only used when doing a full clone and is never returned
	TPM             *TpmState        `json:"tpm,omitempty"`
	Tablet          *bool            `json:"tablet,omitempty"`
	Tags            *[]Tag           `json:"tags,omitempty"`
	VmID            int              `json:"vmid,omitempty"` // TODO should be a custom type as there are limitations
}

const (
	ConfigQemu_Error_UnableToUpdateWithoutReboot string = "unable to update vm without rebooting"
	ConfigQemu_Error_CpuRequired                 string = "cpu is required during creation"
	ConfigQemu_Error_MemoryRequired              string = "memory is required during creation"
)

// Create - Tell Proxmox API to make the VM
func (config ConfigQemu) Create(vmr *VmRef, client *Client) (err error) {
	_, err = config.setAdvanced(nil, false, vmr, client)
	return
}

func (config *ConfigQemu) defaults() {
	if config == nil {
		return
	}
	if config.Boot == "" {
		config.Boot = "cdn"
	}
	if config.Bios == "" {
		config.Bios = "seabios"
	}
	if config.RNGDrive == nil {
		config.RNGDrive = QemuDevice{}
	}
	if config.EFIDisk == nil {
		config.EFIDisk = QemuDevice{}
	}
	if config.Onboot == nil {
		config.Onboot = util.Pointer(true)
	}
	if config.Hotplug == "" {
		config.Hotplug = "network,disk,usb"
	}
	if config.Protection == nil {
		config.Protection = util.Pointer(false)
	}
	if config.QemuDisks == nil {
		config.QemuDisks = QemuDevices{}
	}
	if config.QemuKVM == nil {
		config.QemuKVM = util.Pointer(true)
	}
	if config.QemuNetworks == nil {
		config.QemuNetworks = QemuDevices{}
	}
	if config.QemuOs == "" {
		config.QemuOs = "other"
	}
	if config.QemuPCIDevices == nil {
		config.QemuPCIDevices = QemuDevices{}
	}
	if config.QemuUnusedDisks == nil {
		config.QemuUnusedDisks = QemuDevices{}
	}
	if config.QemuUsbs == nil {
		config.QemuUsbs = QemuDevices{}
	}
	if config.QemuVga == nil {
		config.QemuVga = QemuDevice{}
	}
	if config.Scsihw == "" {
		config.Scsihw = "lsi"
	}
	if config.Tablet == nil {
		config.Tablet = util.Pointer(true)
	}
}

func (config ConfigQemu) mapToAPI(currentConfig ConfigQemu, version Version) (rebootRequired bool, params map[string]interface{}, err error) {
	// TODO check if cloudInit settings changed, they require a reboot to take effect.
	var itemsToDelete string

	params = map[string]interface{}{}

	if config.VmID != 0 {
		params["vmid"] = config.VmID
	}
	if config.Args != "" {
		params["args"] = config.Args
	}
	if config.Agent != nil {
		params["agent"] = config.Agent.mapToAPI(currentConfig.Agent)
	}
	if config.Bios != "" {
		params["bios"] = config.Bios
	}
	if config.Boot != "" {
		params["boot"] = config.Boot
	}
	if config.Description != nil && (*config.Description != "" || currentConfig.Description != nil) {
		params["description"] = *config.Description
	}
	if config.Hookscript != "" {
		params["hookscript"] = config.Hookscript
	}
	if config.Hotplug != "" {
		params["hotplug"] = config.Hotplug
	}
	if config.QemuKVM != nil {
		params["kvm"] = *config.QemuKVM
	}
	if config.Machine != "" {
		params["machine"] = config.Machine
	}
	if config.Name != "" {
		params["name"] = config.Name
	}
	if config.Onboot != nil {
		params["onboot"] = *config.Onboot
	}
	if config.Protection != nil {
		params["protection"] = *config.Protection
	}
	if config.QemuOs != "" {
		params["ostype"] = config.QemuOs
	}
	if config.Scsihw != "" {
		params["scsihw"] = config.Scsihw
	}
	if config.Startup != "" {
		params["startup"] = config.Startup
	}
	if config.Tablet != nil {
		params["tablet"] = *config.Tablet
	}
	if config.Tags != nil {
		params["tags"] = Tag("").mapToApi(*config.Tags)
	}
	if config.Smbios1 != "" {
		params["smbios1"] = config.Smbios1
	}
	if config.TPM != nil {
		if delete := config.TPM.mapToApi(params, currentConfig.TPM); delete != "" {
			itemsToDelete = AddToList(itemsToDelete, delete)
		}
	}

	if config.Iso != nil {
		if config.Disks == nil {
			config.Disks = &QemuStorages{}
		}
		if config.Disks.Ide == nil {
			config.Disks.Ide = &QemuIdeDisks{}
		}
		if config.Disks.Ide.Disk_2 == nil {
			config.Disks.Ide.Disk_2 = &QemuIdeStorage{}
		}
		if config.Disks.Ide.Disk_2.CdRom == nil {
			config.Disks.Ide.Disk_2.CdRom = &QemuCdRom{Iso: config.Iso}
		}
	}
	// Disks
	if currentConfig.Disks != nil {
		if config.Disks != nil {
			// Create,Update,Delete
			delete := config.Disks.mapToApiValues(*currentConfig.Disks, uint(config.VmID), currentConfig.LinkedVmId, params)
			if delete != "" {
				itemsToDelete = AddToList(itemsToDelete, delete)
			}
		}
	} else {
		if config.Disks != nil {
			// Create
			config.Disks.mapToApiValues(QemuStorages{}, uint(config.VmID), 0, params)
		}
	}

	if config.CPU != nil {
		itemsToDelete += config.CPU.mapToApi(currentConfig.CPU, params, version)
	}
	if config.CloudInit != nil {
		itemsToDelete += config.CloudInit.mapToAPI(currentConfig.CloudInit, params, version)
	}
	if config.Memory != nil {
		itemsToDelete += config.Memory.mapToAPI(currentConfig.Memory, params)
	}
	if config.Serials != nil {
		itemsToDelete += config.Serials.mapToAPI(currentConfig.Serials, params)
	}

	// Create EFI disk
	config.CreateQemuEfiParams(params)

	// Create VirtIO RNG
	config.CreateQemuRngParams(params)

	// Create networks config.
	config.CreateQemuNetworksParams(params)

	// Create vga config.
	vgaParam := QemuDeviceParam{}
	vgaParam = vgaParam.createDeviceParam(config.QemuVga, nil)
	if len(vgaParam) > 0 {
		params["vga"] = strings.Join(vgaParam, ",")
	}

	// Create usb interfaces
	config.CreateQemuUsbsParams(params)

	config.CreateQemuPCIsParams(params)

	if itemsToDelete != "" {
		params["delete"] = strings.TrimPrefix(itemsToDelete, ",")
	}
	return
}

func (ConfigQemu) mapToStruct(vmr *VmRef, params map[string]interface{}) (*ConfigQemu, error) {
	// vmConfig Sample: map[ cpu:host
	// net0:virtio=62:DF:XX:XX:XX:XX,bridge=vmbr0
	// ide2:local:iso/xxx-xx.iso,media=cdrom memory:2048
	// smbios1:uuid=8b3bf833-aad8-4545-xxx-xxxxxxx digest:aa6ce5xxxxx1b9ce33e4aaeff564d4 sockets:1
	// name:terraform-ubuntu1404-template bootdisk:virtio0
	// virtio0:ProxmoxxxxISCSI:vm-1014-disk-2,size=4G
	// description:Base image
	// cores:2 ostype:l26

	config := ConfigQemu{
		CPU:       QemuCPU{}.mapToSDK(params),
		CloudInit: CloudInit{}.mapToSDK(params),
		Memory:    QemuMemory{}.mapToSDK(params),
	}

	if vmr != nil {
		config.Node = vmr.node
		poolCopy := PoolName(vmr.pool)
		config.Pool = &poolCopy
		config.VmID = vmr.vmId
	}

	if v, isSet := params["agent"]; isSet {
		config.Agent = QemuGuestAgent{}.mapToSDK(v.(string))
	}
	if _, isSet := params["args"]; isSet {
		config.Args = strings.TrimSpace(params["args"].(string))
	}
	//boot by default from hard disk (c), CD-ROM (d), network (n).
	if _, isSet := params["boot"]; isSet {
		config.Boot = params["boot"].(string)
	}
	if _, isSet := params["bootdisk"]; isSet {
		config.BootDisk = params["bootdisk"].(string)
	}
	if _, isSet := params["bios"]; isSet {
		config.Bios = params["bios"].(string)
	}
	if _, isSet := params["description"]; isSet {
		tmp := params["description"].(string)
		config.Description = &tmp
	}
	//Can be network,disk,cpu,memory,usb
	if _, isSet := params["hotplug"]; isSet {
		config.Hotplug = params["hotplug"].(string)
	}
	if _, isSet := params["hookscript"]; isSet {
		config.Hookscript = params["hookscript"].(string)
	}
	if _, isSet := params["machine"]; isSet {
		config.Machine = params["machine"].(string)
	}
	if _, isSet := params["name"]; isSet {
		config.Name = params["name"].(string)
	}
	if _, isSet := params["onboot"]; isSet {
		config.Onboot = util.Pointer(Itob(int(params["onboot"].(float64))))
	}
	if itemValue, isSet := params["tpmstate0"]; isSet {
		config.TPM = TpmState{}.mapToSDK(itemValue.(string))
	}
	if _, isSet := params["kvm"]; isSet {
		config.QemuKVM = util.Pointer(Itob(int(params["kvm"].(float64))))
	}
	if _, isSet := params["ostype"]; isSet {
		config.QemuOs = params["ostype"].(string)
	}
	if _, isSet := params["protection"]; isSet {
		config.Protection = util.Pointer(Itob(int(params["protection"].(float64))))
	}
	if _, isSet := params["scsihw"]; isSet {
		config.Scsihw = params["scsihw"].(string)
	}
	if _, isSet := params["startup"]; isSet {
		config.Startup = params["startup"].(string)
	}
	if _, isSet := params["tablet"]; isSet {
		config.Tablet = util.Pointer(Itob(int(params["tablet"].(float64))))
	}
	if _, isSet := params["tags"]; isSet {
		tmpTags := Tag("").mapToSDK(params["tags"].(string))
		config.Tags = &tmpTags
	}
	if _, isSet := params["smbios1"]; isSet {
		config.Smbios1 = params["smbios1"].(string)
	}

	linkedVmId := uint(0)
	config.Disks = QemuStorages{}.mapToStruct(params, &linkedVmId)
	if linkedVmId != 0 {
		config.LinkedVmId = linkedVmId
	}

	if config.Disks != nil && config.Disks.Ide != nil && config.Disks.Ide.Disk_2 != nil && config.Disks.Ide.Disk_2.CdRom != nil {
		config.Iso = config.Disks.Ide.Disk_2.CdRom.Iso
	}

	// Add unused disks
	// unused0:local:100/vm-100-disk-1.qcow2
	unusedDiskNames := []string{}
	for k := range params {
		// look for entries from the config in the format "unusedX:<storagepath>" where X is an integer
		if unusedDiskName := rxUnusedDiskName.FindStringSubmatch(k); len(unusedDiskName) > 0 {
			unusedDiskNames = append(unusedDiskNames, unusedDiskName[0])
		}
	}
	// if len(unusedDiskNames) > 0 {
	// 	log.Printf("[DEBUG] unusedDiskNames: %v", unusedDiskNames)
	// }

	if len(unusedDiskNames) > 0 {
		config.QemuUnusedDisks = QemuDevices{}
		for _, unusedDiskName := range unusedDiskNames {
			unusedDiskConfStr := params[unusedDiskName].(string)
			finalDiskConfMap := QemuDevice{}

			// parse "unused0" to get the id '0' as an int
			id := rxDeviceID.FindStringSubmatch(unusedDiskName)
			diskID, err := strconv.Atoi(id[0])
			if err != nil {
				return nil, fmt.Errorf("unable to parse unused disk id from input string '%v' tried to convert '%v' to integer", unusedDiskName, diskID)
			}
			finalDiskConfMap["slot"] = diskID

			// parse the attributes from the unused disk
			// extract the storage and file path from the unused disk entry
			parsedUnusedDiskMap := ParsePMConf(unusedDiskConfStr, "storage+file")
			storageName, fileName := ParseSubConf(parsedUnusedDiskMap["storage+file"].(string), ":")
			finalDiskConfMap["storage"] = storageName
			finalDiskConfMap["file"] = fileName

			config.QemuUnusedDisks[diskID] = finalDiskConfMap
			config.QemuUnusedDisks[diskID] = finalDiskConfMap
			config.QemuUnusedDisks[diskID] = finalDiskConfMap
		}
	}
	//Display

	if vga, isSet := params["vga"]; isSet {
		vgaList := strings.Split(vga.(string), ",")
		vgaMap := QemuDevice{}

		vgaMap.readDeviceConfig(vgaList)
		if len(vgaMap) > 0 {
			config.QemuVga = vgaMap
		}
	}

	// Add networks.
	nicNames := []string{}

	for k := range params {
		if nicName := rxNicName.FindStringSubmatch(k); len(nicName) > 0 {
			nicNames = append(nicNames, nicName[0])
		}
	}

	if len(nicNames) > 0 {
		config.QemuNetworks = QemuDevices{}
		for _, nicName := range nicNames {
			nicConfStr := params[nicName]
			nicConfList := strings.Split(nicConfStr.(string), ",")

			id := rxDeviceID.FindStringSubmatch(nicName)
			nicID, _ := strconv.Atoi(id[0])
			model, macaddr := ParseSubConf(nicConfList[0], "=")

			// Add model and MAC address.
			nicConfMap := QemuDevice{
				"id":      nicID,
				"model":   model,
				"macaddr": macaddr,
			}

			// Add rest of device config.
			nicConfMap.readDeviceConfig(nicConfList[1:])
			switch nicConfMap["firewall"] {
			case 1:
				nicConfMap["firewall"] = true
			case 0:
				nicConfMap["firewall"] = false
			}
			switch nicConfMap["link_down"] {
			case 1:
				nicConfMap["link_down"] = true
			case 0:
				nicConfMap["link_down"] = false
			}

			// And device config to networks.
			if len(nicConfMap) > 0 {
				config.QemuNetworks[nicID] = nicConfMap
			}
		}
	}

	config.Serials = SerialInterfaces{}.mapToSDK(params)

	// Add usbs
	usbNames := []string{}

	for k := range params {
		if usbName := rxUsbName.FindStringSubmatch(k); len(usbName) > 0 {
			usbNames = append(usbNames, usbName[0])
		}
	}

	if len(usbNames) > 0 {
		config.QemuUsbs = QemuDevices{}
		for _, usbName := range usbNames {
			usbConfStr := params[usbName]
			usbConfList := strings.Split(usbConfStr.(string), ",")
			id := rxDeviceID.FindStringSubmatch(usbName)
			usbID, _ := strconv.Atoi(id[0])
			_, host := ParseSubConf(usbConfList[0], "=")

			usbConfMap := QemuDevice{
				"id":   usbID,
				"host": host,
			}

			usbConfMap.readDeviceConfig(usbConfList[1:])
			if usbConfMap["usb3"] == 1 {
				usbConfMap["usb3"] = true
			}

			// And device config to usbs map.
			if len(usbConfMap) > 0 {
				config.QemuUsbs[usbID] = usbConfMap
			}
		}
	}

	// hostpci
	hostPCInames := []string{}

	for k := range params {
		if hostPCIname := rxPCIName.FindStringSubmatch(k); len(hostPCIname) > 0 {
			hostPCInames = append(hostPCInames, hostPCIname[0])
		}
	}

	if len(hostPCInames) > 0 {
		config.QemuPCIDevices = QemuDevices{}
		for _, hostPCIname := range hostPCInames {
			hostPCIConfStr := params[hostPCIname]
			hostPCIConfList := strings.Split(hostPCIConfStr.(string), ",")
			id := rxPCIName.FindStringSubmatch(hostPCIname)
			hostPCIID, _ := strconv.Atoi(id[0])
			hostPCIConfMap := QemuDevice{
				"id": hostPCIID,
			}
			hostPCIConfMap.readDeviceConfig(hostPCIConfList)
			// And device config to usbs map.
			if len(hostPCIConfMap) > 0 {
				config.QemuPCIDevices[hostPCIID] = hostPCIConfMap
			}
		}
	}

	// efidisk
	if efidisk, isSet := params["efidisk0"].(string); isSet {
		efiDiskConfMap := ParsePMConf(efidisk, "volume")
		storageName, fileName := ParseSubConf(efiDiskConfMap["volume"].(string), ":")
		efiDiskConfMap["storage"] = storageName
		efiDiskConfMap["file"] = fileName
		config.EFIDisk = efiDiskConfMap
	}

	return &config, nil
}

func (newConfig ConfigQemu) Update(rebootIfNeeded bool, vmr *VmRef, client *Client) (rebootRequired bool, err error) {
	currentConfig, err := NewConfigQemuFromApi(vmr, client)
	if err != nil {
		return
	}
	return newConfig.setAdvanced(currentConfig, rebootIfNeeded, vmr, client)
}

func (config *ConfigQemu) setVmr(vmr *VmRef) (err error) {
	if config == nil {
		return errors.New("config may not be nil")
	}
	if err = vmr.nilCheck(); err != nil {
		return
	}
	vmr.SetVmType("qemu")
	config.VmID = vmr.vmId
	config.Node = vmr.node
	return
}

// currentConfig will be mutated
func (newConfig ConfigQemu) setAdvanced(currentConfig *ConfigQemu, rebootIfNeeded bool, vmr *VmRef, client *Client) (rebootRequired bool, err error) {
	if err = newConfig.setVmr(vmr); err != nil {
		return
	}
	var version Version
	if version, err = client.Version(); err != nil {
		return
	}
	if err = newConfig.Validate(currentConfig, version); err != nil {
		return
	}

	var params map[string]interface{}
	var exitStatus string

	if currentConfig != nil { // Update
		// TODO implement tmp move and version change
		url := "/nodes/" + vmr.node + "/" + vmr.vmType + "/" + strconv.Itoa(vmr.vmId) + "/config"
		var itemsToDeleteBeforeUpdate string // this is for items that should be removed before they can be created again e.g. cloud-init disks. (convert to array when needed)
		stopped := false

		var markedDisks qemuUpdateChanges
		if newConfig.Disks != nil && currentConfig.Disks != nil {
			markedDisks = *newConfig.Disks.markDiskChanges(*currentConfig.Disks)
			for _, e := range markedDisks.Move { // move disk to different storage or change disk format
				_, err = e.move(true, vmr, client)
				if err != nil {
					return
				}
			}
			if err = resizeDisks(vmr, client, markedDisks.Resize); err != nil { // increase Disks in size
				return false, err
			}
			itemsToDeleteBeforeUpdate = newConfig.Disks.cloudInitRemove(*currentConfig.Disks)
		}

		if newConfig.TPM != nil && currentConfig.TPM != nil { // delete or move TPM
			delete, disk := newConfig.TPM.markChanges(*currentConfig.TPM)
			if delete != "" { // delete
				itemsToDeleteBeforeUpdate = AddToList(itemsToDeleteBeforeUpdate, delete)
				currentConfig.TPM = nil
			} else if disk != nil { // move
				if _, err := disk.move(true, vmr, client); err != nil {
					return false, err
				}
			}
		}

		if itemsToDeleteBeforeUpdate != "" {
			err = client.Put(map[string]interface{}{"delete": itemsToDeleteBeforeUpdate}, url)
			if err != nil {
				return false, fmt.Errorf("error updating VM: %v", err)
			}
			// Deleteing these items can create pending changes
			rebootRequired, err = GuestHasPendingChanges(vmr, client)
			if err != nil {
				return
			}
			if rebootRequired { // shutdown vm if reboot is required
				if rebootIfNeeded {
					if err = GuestShutdown(vmr, client, true); err != nil {
						return
					}
					stopped = true
					rebootRequired = false
				} else {
					return rebootRequired, errors.New(ConfigQemu_Error_UnableToUpdateWithoutReboot)
				}
			}
		}

		// TODO GuestHasPendingChanges() has the current vm config technically. We can use this to avoid an extra API call.
		if len(markedDisks.Move) != 0 { // Moving disks changes the disk id. we need to get the config again if any disk was moved.
			currentConfig, err = NewConfigQemuFromApi(vmr, client)
			if err != nil {
				return
			}
		}

		if newConfig.Node != currentConfig.Node { // Migrate VM
			vmr.SetNode(currentConfig.Node)
			_, err = client.MigrateNode(vmr, newConfig.Node, true)
			if err != nil {
				return
			}
			// Set node to the node the VM was migrated to
			vmr.SetNode(newConfig.Node)
		}

		rebootRequired, params, err = newConfig.mapToAPI(*currentConfig, version)
		if err != nil {
			return
		}
		exitStatus, err = client.PutWithTask(params, url)
		if err != nil {
			return false, fmt.Errorf("error updating VM: %v, error status: %s (params: %v)", err, exitStatus, params)
		}

		if !rebootRequired && !stopped { // only check if reboot is required if the vm is not already stopped
			rebootRequired, err = GuestHasPendingChanges(vmr, client)
			if err != nil {
				return
			}
		}

		if err = resizeNewDisks(vmr, client, newConfig.Disks, currentConfig.Disks); err != nil {
			return
		}

		if newConfig.Pool != nil { // update pool membership
			guestSetPool_Unsafe(client, uint(vmr.vmId), *newConfig.Pool, currentConfig.Pool, version)
		}

		if stopped { // start vm if it was stopped
			if rebootIfNeeded {
				if err = GuestStart(vmr, client); err != nil {
					return
				}
				stopped = false
				rebootRequired = false
			} else {
				return true, nil
			}
		} else if rebootRequired { // reboot vm if it is running
			if rebootIfNeeded {
				if err = GuestReboot(vmr, client); err != nil {
					return
				}
				rebootRequired = false
			} else {
				return rebootRequired, nil
			}
		}
	} else { // Create
		_, params, err = newConfig.mapToAPI(ConfigQemu{}, version)
		if err != nil {
			return
		}
		exitStatus, err = client.CreateQemuVm(vmr.node, params)
		if err != nil {
			return false, fmt.Errorf("error creating VM: %v, error status: %s (params: %v)", err, exitStatus, params)
		}
		if err = resizeNewDisks(vmr, client, newConfig.Disks, nil); err != nil {
			return
		}
		if newConfig.Pool != nil && *newConfig.Pool != "" { // add guest to pool
			if err = newConfig.Pool.addGuests_Unsafe(client, []uint{uint(vmr.vmId)}, nil, version); err != nil {
				return
			}
		}
		if err = client.insertCachedPermission(permissionPath(permissionCategory_GuestPath) + "/" + permissionPath(strconv.Itoa(vmr.vmId))); err != nil {
			return
		}
	}

	_, err = client.UpdateVMHA(vmr, newConfig.HaState, newConfig.HaGroup)
	return
}

func (config ConfigQemu) Validate(current *ConfigQemu, version Version) (err error) {
	// TODO test all other use cases
	// TODO has no context about changes caused by updating the vm
	if current == nil { // Create
		if config.CPU == nil {
			return errors.New(ConfigQemu_Error_CpuRequired)
		} else {
			if err = config.CPU.Validate(nil, version); err != nil {
				return
			}
		}
		if config.Memory == nil {
			return errors.New(ConfigQemu_Error_MemoryRequired)
		} else {
			if err = config.Memory.Validate(nil); err != nil {
				return
			}
		}
		if config.TPM != nil {
			if err = config.TPM.Validate(nil); err != nil {
				return
			}
		}
	} else { // Update
		if config.CPU != nil {
			if err = config.CPU.Validate(current.CPU, version); err != nil {
				return
			}
		}
		if config.Memory != nil {
			if err = config.Memory.Validate(current.Memory); err != nil {
				return
			}
		}
		if config.TPM != nil {
			if err = config.TPM.Validate(current.TPM); err != nil {
				return
			}
		}
	}
	// Shared
	if config.Agent != nil {
		if err = config.Agent.Validate(); err != nil {
			return
		}
	}
	if config.CloudInit != nil {
		if err = config.CloudInit.Validate(version); err != nil {
			return
		}
	}
	if config.Disks != nil {
		err = config.Disks.Validate()
		if err != nil {
			return
		}
	}
	if config.Pool != nil && *config.Pool != "" {
		if err = config.Pool.Validate(); err != nil {
			return
		}
	}
	if len(config.Serials) > 0 {
		if err = config.Serials.Validate(); err != nil {
			return
		}
	}
	if config.Tags != nil {
		if err := Tag("").validate(*config.Tags); err != nil {
			return err
		}
	}

	return
}

/*
CloneVm
Example: Request

nodes/proxmox1-xx/qemu/1012/clone

newid:145
name:tf-clone1
target:proxmox1-xx
full:1
storage:xxx
*/
func (config ConfigQemu) CloneVm(sourceVmr *VmRef, vmr *VmRef, client *Client) (err error) {
	vmr.SetVmType("qemu")
	var storage string
	fullClone := "1"
	if config.FullClone != nil {
		fullClone = strconv.Itoa(*config.FullClone)
	}
	if disk0Storage, ok := config.QemuDisks[0]["storage"].(string); ok && len(disk0Storage) > 0 {
		storage = disk0Storage
	}
	params := map[string]interface{}{
		"newid":  vmr.vmId,
		"target": vmr.node,
		"name":   config.Name,
		"full":   fullClone,
	}
	if vmr.pool != "" {
		params["pool"] = vmr.pool
	}

	if fullClone == "1" && storage != "" {
		params["storage"] = storage
	}

	_, err = client.CloneQemuVm(sourceVmr, params)
	return err
}

func NewConfigQemuFromJson(input []byte) (config *ConfigQemu, err error) {
	config = &ConfigQemu{}
	err = json.Unmarshal([]byte(input), config)
	if err != nil {
		log.Fatal(err)
	}
	return
}

var (
	rxDeviceID       = regexp.MustCompile(`\d+`)
	rxUnusedDiskName = regexp.MustCompile(`^(unused)\d+`)
	rxNicName        = regexp.MustCompile(`net\d+`)
	rxMpName         = regexp.MustCompile(`mp\d+`)
	rxUsbName        = regexp.MustCompile(`usb\d+`)
	rxPCIName        = regexp.MustCompile(`hostpci\d+`)
)

func NewConfigQemuFromApi(vmr *VmRef, client *Client) (config *ConfigQemu, err error) {
	var vmConfig map[string]interface{}
	var vmInfo map[string]interface{}
	for ii := 0; ii < 3; ii++ {
		vmConfig, err = client.GetVmConfig(vmr)
		if err != nil {
			log.Fatal(err)
			return nil, err
		}
		// TODO: this is a workaround for the issue that GetVmConfig will not always return the guest info
		vmInfo, err = client.GetVmInfo(vmr)
		if err != nil {
			return nil, err
		}
		// this can happen:
		// {"data":{"lock":"clone","digest":"eb54fb9d9f120ba0c3bdf694f73b10002c375c38","description":" qmclone temporary file\n"}})
		if vmInfo["lock"] == nil {
			break
		} else {
			time.Sleep(8 * time.Second)
		}
	}

	if vmInfo["lock"] != nil {
		return nil, fmt.Errorf("vm locked, could not obtain config")
	}
	if v, isSet := vmInfo["pool"]; isSet { // TODO: this is a workaround for the issue that GetVmConfig will not always return the guest info
		vmr.pool = v.(string)
	}
	config, err = ConfigQemu{}.mapToStruct(vmr, vmConfig)
	if err != nil {
		return
	}

	config.defaults()

	// HAstate is return by the api for a vm resource type but not the HAgroup
	err = client.ReadVMHA(vmr) // TODO: can be optimized, uses same API call as GetVmConfig and GetVmInfo
	if err == nil {
		config.HaState = vmr.HaState()
		config.HaGroup = vmr.HaGroup()
	} else {
		//log.Printf("[DEBUG] VM %d(%s) has no HA config", vmr.vmId, vmConfig["hostname"])
		return config, nil
	}
	return
}

// Useful waiting for ISO install to complete
func WaitForShutdown(vmr *VmRef, client *Client) (err error) {
	for ii := 0; ii < 100; ii++ {
		vmState, err := client.GetVmState(vmr)
		if err != nil {
			log.Print("Wait error:")
			log.Println(err)
		} else if vmState["status"] == "stopped" {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("not shutdown within wait time")
}

// This is because proxmox create/config API won't let us make usernet devices
func SshForwardUsernet(vmr *VmRef, client *Client) (sshPort string, err error) {
	vmState, err := client.GetVmState(vmr)
	if err != nil {
		return "", err
	}
	if vmState["status"] == "stopped" {
		return "", fmt.Errorf("VM must be running first")
	}
	sshPort = strconv.Itoa(vmr.VmId() + 22000)
	_, err = client.MonitorCmd(vmr, "netdev_add user,id=net1,hostfwd=tcp::"+sshPort+"-:22")
	if err != nil {
		return "", err
	}
	_, err = client.MonitorCmd(vmr, "device_add virtio-net-pci,id=net1,netdev=net1,addr=0x13")
	if err != nil {
		return "", err
	}
	return
}

// device_del net1
// netdev_del net1
func RemoveSshForwardUsernet(vmr *VmRef, client *Client) (err error) {
	vmState, err := client.GetVmState(vmr)
	if err != nil {
		return err
	}
	if vmState["status"] == "stopped" {
		return fmt.Errorf("VM must be running first")
	}
	_, err = client.MonitorCmd(vmr, "device_del net1")
	if err != nil {
		return err
	}
	_, err = client.MonitorCmd(vmr, "netdev_del net1")
	if err != nil {
		return err
	}
	return nil
}

func MaxVmId(client *Client) (max int, err error) {
	vms, err := client.GetResourceList(resourceListGuest)
	max = 100
	for vmii := range vms {
		vm := vms[vmii].(map[string]interface{})
		vmid := int(vm["vmid"].(float64))
		if vmid > max {
			max = vmid
		}
	}
	return
}

func SendKeysString(vmr *VmRef, client *Client, keys string) (err error) {
	vmState, err := client.GetVmState(vmr)
	if err != nil {
		return err
	}
	if vmState["status"] == "stopped" {
		return fmt.Errorf("VM must be running first")
	}
	for _, r := range keys {
		c := string(r)
		lower := strings.ToLower(c)
		if c != lower {
			c = "shift-" + lower
		} else {
			switch c {
			case "!":
				c = "shift-1"
			case "@":
				c = "shift-2"
			case "#":
				c = "shift-3"
			case "$":
				c = "shift-4"
			case "%%":
				c = "shift-5"
			case "^":
				c = "shift-6"
			case "&":
				c = "shift-7"
			case "*":
				c = "shift-8"
			case "(":
				c = "shift-9"
			case ")":
				c = "shift-0"
			case "_":
				c = "shift-minus"
			case "+":
				c = "shift-equal"
			case " ":
				c = "spc"
			case "/":
				c = "slash"
			case "\\":
				c = "backslash"
			case ",":
				c = "comma"
			case "-":
				c = "minus"
			case "=":
				c = "equal"
			case ".":
				c = "dot"
			case "?":
				c = "shift-slash"
			}
		}
		_, err = client.MonitorCmd(vmr, "sendkey "+c)
		if err != nil {
			return err
		}
		time.Sleep(1 * time.Millisecond)
	}
	return nil
}

// Given a QemuDevice, return a param string to give to ProxMox
func formatDeviceParam(device QemuDevice) string {
	deviceConfParams := QemuDeviceParam{}
	deviceConfParams = deviceConfParams.createDeviceParam(device, nil)
	return strings.Join(deviceConfParams, ",")
}

// Given a QemuDevice (representing a disk), return a param string to give to ProxMox
func FormatDiskParam(disk QemuDevice) string {
	diskConfParam := QemuDeviceParam{}

	if volume, ok := disk["volume"]; ok && volume != "" {
		diskConfParam = append(diskConfParam, volume.(string))

		if size, ok := disk["size"]; ok && size != "" {
			diskConfParam = append(diskConfParam, fmt.Sprintf("size=%v", disk["size"]))
		}
	} else {
		volumeInit := fmt.Sprintf("%v:%v", disk["storage"], DiskSizeGB(disk["size"]))
		diskConfParam = append(diskConfParam, volumeInit)
	}

	// Set cache if not none (default).
	if cache, ok := disk["cache"]; ok && cache != "none" {
		diskCache := fmt.Sprintf("cache=%v", disk["cache"])
		diskConfParam = append(diskConfParam, diskCache)
	}

	// Mountoptions
	if mountoptions, ok := disk["mountoptions"]; ok {
		options := []string{}
		for opt, enabled := range mountoptions.(map[string]interface{}) {
			if enabled.(bool) {
				options = append(options, opt)
			}
		}
		diskMountOpts := fmt.Sprintf("mountoptions=%v", strings.Join(options, ";"))
		diskConfParam = append(diskConfParam, diskMountOpts)
	}

	// Backup
	if backup, ok := disk["backup"].(bool); ok {
		// Backups are enabled by default (backup=1)
		// Only set the parameter if backups are explicitly disabled
		if !backup {
			diskConfParam = append(diskConfParam, "backup=0")
		}
	}

	// Keys that are not used as real/direct conf.
	ignoredKeys := []string{"backup", "key", "slot", "type", "storage", "file", "size", "cache", "volume", "container", "vm", "mountoptions", "storage_type"}

	// Rest of config.
	diskConfParam = diskConfParam.createDeviceParam(disk, ignoredKeys)

	return strings.Join(diskConfParam, ",")
}

// Given a QemuDevice (representing a usb), return a param string to give to ProxMox
func FormatUsbParam(usb QemuDevice) string {
	usbConfParam := QemuDeviceParam{}

	usbConfParam = usbConfParam.createDeviceParam(usb, []string{})

	return strings.Join(usbConfParam, ",")
}

// Create parameters for each Nic device.
func (c ConfigQemu) CreateQemuNetworksParams(params map[string]interface{}) {
	// For new style with multi net device.
	for nicID, nicConfMap := range c.QemuNetworks {

		nicConfParam := QemuDeviceParam{}

		// Set Nic name.
		qemuNicName := "net" + strconv.Itoa(nicID)

		// Set Mac address.
		var macAddr string
		switch nicConfMap["macaddr"] {
		case nil, "":
			// Generate random Mac based on time
			macaddr := make(net.HardwareAddr, 6)
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			r.Read(macaddr)
			macaddr[0] = (macaddr[0] | 2) & 0xfe // fix from github issue #18
			macAddr = strings.ToUpper(fmt.Sprintf("%v", macaddr))

			// Add Mac to source map so it will be returned. (useful for some use case like Terraform)
			nicConfMap["macaddr"] = macAddr
		case "repeatable":
			// Generate deterministic Mac based on VmID and NicID
			// Assume that rare VM has more than 32 nics
			macaddr := make(net.HardwareAddr, 6)
			pairing := c.VmID<<5 | nicID
			// Linux MAC vendor - 00:18:59
			macaddr[0] = 0x00
			macaddr[1] = 0x18
			macaddr[2] = 0x59
			macaddr[3] = byte((pairing >> 16) & 0xff)
			macaddr[4] = byte((pairing >> 8) & 0xff)
			macaddr[5] = byte(pairing & 0xff)
			// Convert to string
			macAddr = strings.ToUpper(fmt.Sprintf("%v", macaddr))

			// Add Mac to source map so it will be returned. (useful for some use case like Terraform)
			nicConfMap["macaddr"] = macAddr
		default:
			macAddr = nicConfMap["macaddr"].(string)
		}

		// use model=mac format for older proxmox compatibility as the parameters which will be sent to Proxmox API.
		nicConfParam = append(nicConfParam, fmt.Sprintf("%v=%v", nicConfMap["model"], macAddr))

		// Set bridge if not nat.
		if nicConfMap["bridge"].(string) != "nat" {
			bridge := fmt.Sprintf("bridge=%v", nicConfMap["bridge"])
			nicConfParam = append(nicConfParam, bridge)
		}

		// Keys that are not used as real/direct conf.
		ignoredKeys := []string{"id", "bridge", "macaddr", "model"}

		// Rest of config.
		nicConfParam = nicConfParam.createDeviceParam(nicConfMap, ignoredKeys)

		// Add nic to Qemu prams.
		params[qemuNicName] = strings.Join(nicConfParam, ",")
	}
}

// Create RNG parameter.
func (c ConfigQemu) CreateQemuRngParams(params map[string]interface{}) {
	rngParam := QemuDeviceParam{}
	rngParam = rngParam.createDeviceParam(c.RNGDrive, nil)

	if len(rngParam) > 0 {
		rng_info := []string{}
		rng := ""
		for _, param := range rngParam {
			key := strings.Split(param, "=")
			rng_info = append(rng_info, fmt.Sprintf("%s=%s", key[0], key[1]))
		}
		if len(rng_info) > 0 {
			rng = strings.Join(rng_info, ",")
			params["rng0"] = rng
		}
	}
}

// Create efi parameter.
func (c ConfigQemu) CreateQemuEfiParams(params map[string]interface{}) {
	efiParam := QemuDeviceParam{}
	efiParam = efiParam.createDeviceParam(c.EFIDisk, nil)

	if len(efiParam) > 0 {
		storage_info := []string{}
		storage := ""
		for _, param := range efiParam {
			key := strings.Split(param, "=")
			if key[0] == "storage" {
				// Proxmox format for disk creation
				storage = fmt.Sprintf("%s:1", key[1])
			} else {
				storage_info = append(storage_info, param)
			}
		}
		if len(storage_info) > 0 {
			storage = fmt.Sprintf("%s,%s", storage, strings.Join(storage_info, ","))
		}
		params["efidisk0"] = storage
	}
}

// Create parameters for each disk.
func (c ConfigQemu) CreateQemuDisksParams(params map[string]interface{}, cloned bool) {
	// For new style with multi disk device.
	for diskID, diskConfMap := range c.QemuDisks {
		// skip the first disk for clones (may not always be right, but a template probably has at least 1 disk)
		if diskID == 0 && cloned {
			continue
		}

		// Device name.
		deviceType := diskConfMap["type"].(string)
		qemuDiskName := deviceType + strconv.Itoa(diskID)

		// Add back to Qemu prams.
		params[qemuDiskName] = FormatDiskParam(diskConfMap)
	}
}

// Create parameters for each PCI Device
func (c ConfigQemu) CreateQemuPCIsParams(params map[string]interface{}) {
	// For new style with multi pci device.
	for pciConfID, pciConfMap := range c.QemuPCIDevices {
		qemuPCIName := "hostpci" + strconv.Itoa(pciConfID)
		var pcistring bytes.Buffer
		for elem := range pciConfMap {
			pcistring.WriteString(elem)
			pcistring.WriteString("=")
			pcistring.WriteString(fmt.Sprintf("%v", pciConfMap[elem]))
			pcistring.WriteString(",")
		}

		// Add back to Qemu prams.
		params[qemuPCIName] = strings.TrimSuffix(pcistring.String(), ",")
	}
}

// Create parameters for usb interface
func (c ConfigQemu) CreateQemuUsbsParams(params map[string]interface{}) {
	for usbID, usbConfMap := range c.QemuUsbs {
		qemuUsbName := "usb" + strconv.Itoa(usbID)

		// Add back to Qemu prams.
		params[qemuUsbName] = FormatUsbParam(usbConfMap)
	}
}

// Create parameters for serial interface
func (c ConfigQemu) CreateQemuMachineParam(
	params map[string]interface{},
) error {
	if c.Machine == "" {
		return nil
	}
	if matched, _ := regexp.MatchString(machineModels, c.Machine); matched {
		params["machine"] = c.Machine
		return nil
	}
	return fmt.Errorf("unsupported machine type, fall back to default")
}

func (p QemuDeviceParam) createDeviceParam(
	deviceConfMap QemuDevice,
	ignoredKeys []string,
) QemuDeviceParam {

	for key, value := range deviceConfMap {
		if ignored := inArray(ignoredKeys, key); !ignored {
			var confValue interface{}
			if bValue, ok := value.(bool); ok && bValue {
				confValue = "1"
			} else if sValue, ok := value.(string); ok && len(sValue) > 0 {
				confValue = sValue
			} else if iValue, ok := value.(int); ok && iValue > 0 {
				confValue = iValue
			} else if iValue, ok := value.(float64); ok && iValue > 0 {
				confValue = iValue
			}
			if confValue != nil {
				deviceConf := fmt.Sprintf("%v=%v", key, confValue)
				p = append(p, deviceConf)
			}
		}
	}

	return p
}

// readDeviceConfig - get standard sub-conf strings where `key=value` and update conf map.
func (confMap QemuDevice) readDeviceConfig(confList []string) {
	// Add device config.
	for _, conf := range confList {
		key, value := ParseSubConf(conf, "=")
		confMap[key] = value
	}
}

func (c ConfigQemu) String() string {
	jsConf, _ := json.Marshal(c)
	return string(jsConf)
}

type QemuNetworkInterfaceID uint8

const (
	QemuNetworkInterfaceID_Error_Invalid string = "network interface ID must be in the range 0-31"

	QemuNetworkInterfaceID0  QemuNetworkInterfaceID = 0
	QemuNetworkInterfaceID1  QemuNetworkInterfaceID = 1
	QemuNetworkInterfaceID2  QemuNetworkInterfaceID = 2
	QemuNetworkInterfaceID3  QemuNetworkInterfaceID = 3
	QemuNetworkInterfaceID4  QemuNetworkInterfaceID = 4
	QemuNetworkInterfaceID5  QemuNetworkInterfaceID = 5
	QemuNetworkInterfaceID6  QemuNetworkInterfaceID = 6
	QemuNetworkInterfaceID7  QemuNetworkInterfaceID = 7
	QemuNetworkInterfaceID8  QemuNetworkInterfaceID = 8
	QemuNetworkInterfaceID9  QemuNetworkInterfaceID = 9
	QemuNetworkInterfaceID10 QemuNetworkInterfaceID = 10
	QemuNetworkInterfaceID11 QemuNetworkInterfaceID = 11
	QemuNetworkInterfaceID12 QemuNetworkInterfaceID = 12
	QemuNetworkInterfaceID13 QemuNetworkInterfaceID = 13
	QemuNetworkInterfaceID14 QemuNetworkInterfaceID = 14
	QemuNetworkInterfaceID15 QemuNetworkInterfaceID = 15
	QemuNetworkInterfaceID16 QemuNetworkInterfaceID = 16
	QemuNetworkInterfaceID17 QemuNetworkInterfaceID = 17
	QemuNetworkInterfaceID18 QemuNetworkInterfaceID = 18
	QemuNetworkInterfaceID19 QemuNetworkInterfaceID = 19
	QemuNetworkInterfaceID20 QemuNetworkInterfaceID = 20
	QemuNetworkInterfaceID21 QemuNetworkInterfaceID = 21
	QemuNetworkInterfaceID22 QemuNetworkInterfaceID = 22
	QemuNetworkInterfaceID23 QemuNetworkInterfaceID = 23
	QemuNetworkInterfaceID24 QemuNetworkInterfaceID = 24
	QemuNetworkInterfaceID25 QemuNetworkInterfaceID = 25
	QemuNetworkInterfaceID26 QemuNetworkInterfaceID = 26
	QemuNetworkInterfaceID27 QemuNetworkInterfaceID = 27
	QemuNetworkInterfaceID28 QemuNetworkInterfaceID = 28
	QemuNetworkInterfaceID29 QemuNetworkInterfaceID = 29
	QemuNetworkInterfaceID30 QemuNetworkInterfaceID = 30
	QemuNetworkInterfaceID31 QemuNetworkInterfaceID = 31
)

func (id QemuNetworkInterfaceID) Validate() error {
	if id > 31 {
		return errors.New(QemuNetworkInterfaceID_Error_Invalid)
	}
	return nil
}
