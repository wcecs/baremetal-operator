package ironic

import (
	"fmt"
	

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/wcecs/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/wcecs/baremetal-operator/pkg/provisioner"
	"github.com/wcecs/baremetal-operator/pkg/provisioner/ironic/devicehints"

	"github.com/pkg/errors"
)



// setTargetRAIDCfg set the RAID settings to the ironic Node for RAID configuration steps
func setTargetRAIDCfg(p *ironicProvisioner, ironicNode *nodes.Node, data provisioner.PrepareData) (err error) {
	var logicalDisks []nodes.LogicalDisk

	// Build target for RAID configuration steps
	logicalDisks, err = BuildTargetRAIDCfg(data.RAIDConfig)
	if len(logicalDisks) == 0 || err != nil {
		return
	}

	// set root volume
	if data.RootDeviceHints == nil {
		logicalDisks[0].IsRootVolume = new(bool)
		*logicalDisks[0].IsRootVolume = true
	} else {
		p.log.Info("rootDeviceHints is used, the first volume of raid will not be set to root")
	}

	// Set target for RAID configuration steps
	return nodes.SetRAIDConfig(
		p.client,
		ironicNode.UUID,
		nodes.RAIDConfigOpts{LogicalDisks: logicalDisks},
	).ExtractErr()
	
}

// BuildTargetRAIDCfg build RAID logical disks, this method doesn't set the root volume
func BuildTargetRAIDCfg(raid *metal3v1alpha1.RAIDConfig) (logicalDisks []nodes.LogicalDisk, err error) {
	// Deal possible panic
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("panic in build RAID settings: %v", r)
		}
	}()

	if raid == nil {
		return
	}

	// build logicalDisks
	if len(raid.HardwareRAIDVolumes) != 0 {
		logicalDisks, err = buildTargetHardwareRAIDCfg(raid.HardwareRAIDVolumes)
	} else if len(raid.SoftwareRAIDVolumes) != 0 {
		logicalDisks, err = buildTargetSoftwareRAIDCfg(raid.SoftwareRAIDVolumes)
	}

	return
}

// A private method to build hardware RAID disks
func buildTargetHardwareRAIDCfg(volumes []metal3v1alpha1.HardwareRAIDVolume) (logicalDisks []nodes.LogicalDisk, err error) {
	var (
		logicalDisk    nodes.LogicalDisk
		nameCheckFlags map[string]int = make(map[string]int)
	)

	if len(volumes) == 0 {
		return
	}

	for index, volume := range volumes {
		// Check volume's name
		if volume.Name != "" {
			i, exist := nameCheckFlags[volume.Name]
			if exist {
				return nil, errors.Errorf("the names(%s) of volume[%d] and volume[%d] are repeated", volume.Name, index, i)
			}
			nameCheckFlags[volume.Name] = index
		}
		// Build logicalDisk
		logicalDisk = nodes.LogicalDisk{
			SizeGB:     volume.SizeGibibytes,
			RAIDLevel:  nodes.RAIDLevel(volume.Level),
			VolumeName: volume.Name,
		}
		if volume.Rotational != nil {
			if *volume.Rotational {
				logicalDisk.DiskType = nodes.HDD
			} else {
				logicalDisk.DiskType = nodes.SSD
			}
		}
		if volume.NumberOfPhysicalDisks != nil {
			logicalDisk.NumberOfPhysicalDisks = *volume.NumberOfPhysicalDisks
		}
		// Add to logicalDisks
		logicalDisks = append(logicalDisks, logicalDisk)
	}

	return
}

// A private method to build software RAID disks
func buildTargetSoftwareRAIDCfg(volumes []metal3v1alpha1.SoftwareRAIDVolume) (logicalDisks []nodes.LogicalDisk, err error) {
	var (
		logicalDisk nodes.LogicalDisk
	)

	if len(volumes) == 0 {
		return
	}

	if nodes.RAIDLevel(volumes[0].Level) != nodes.RAID1 {
		return nil, errors.Errorf("the level in first volume of software raid must be RAID1")
	}

	for _, volume := range volumes {
		// Build logicalDisk
		logicalDisk = nodes.LogicalDisk{
			SizeGB:     volume.SizeGibibytes,
			RAIDLevel:  nodes.RAIDLevel(volume.Level),
			Controller: "software",
		}
		// Build physical disks hint
		for i := range volume.PhysicalDisks {
			logicalDisk.PhysicalDisks = append(logicalDisk.PhysicalDisks, devicehints.MakeHintMap(&volume.PhysicalDisks[i]))
		}
		// Add to logicalDisks
		logicalDisks = append(logicalDisks, logicalDisk)
	}

	return
}

// BuildRAIDCleanSteps build the clean steps for RAID configuration from BaremetalHost spec
func BuildRAIDCleanSteps(raid *metal3v1alpha1.RAIDConfig) (cleanSteps []nodes.CleanStep) {
	// Add ‘delete_configuration’ before ‘create_configuration’ to make sure
	// that only the desired logical disks exist in the system after manual cleaning.
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "raid",
			Step:      "delete_configuration",
		},
	)
	// If not configure raid, only need to clear old configuration
	if raid == nil || (len(raid.HardwareRAIDVolumes) == 0 && len(raid.SoftwareRAIDVolumes) == 0) {
		return
	}
	if len(raid.HardwareRAIDVolumes) == 0 && len(raid.SoftwareRAIDVolumes) != 0 {
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: "deploy",
				Step:      "erase_devices_metadata",
			},
		)
	}
	// ‘create_configuration’ doesn’t remove existing disks. It is recommended
	// that only the desired logical disks exist in the system after manual cleaning.
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "raid",
			Step:      "create_configuration",
		},
	)
	return
}

		
	
	
	
	


