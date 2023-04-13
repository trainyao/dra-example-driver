/*
 * Copyright 2023 The Kubernetes Authors.
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
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	cdiapi "github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	cdispec "github.com/container-orchestrated-devices/container-device-interface/specs-go"
	nascrd "github.com/kubernetes-sigs/dra-example-driver/api/example.com/resource/gpu/nas/v1alpha1"
)

const (
	cdiVendor = "k8s." + DriverName
	cdiClass  = "gpu"
	cdiKind   = cdiVendor + "/" + cdiClass

	cdiCommonDeviceName = "common"
)

type CDIHandler struct {
	logger   *logrus.Logger
	registry cdiapi.Registry
}

func NewCDIHandler(config *Config) (*CDIHandler, error) {
	registry := cdiapi.GetRegistry(
		cdiapi.WithSpecDirs(*config.flags.cdiRoot),
	)

	err := registry.Refresh()
	if err != nil {
		return nil, fmt.Errorf("unable to refresh the CDI registry: %v", err)
	}

	logger := logrus.New()
	logger.Out = os.Stdout
	logger.Level = logrus.DebugLevel

	handler := &CDIHandler{
		logger:   logger,
		registry: registry,
	}

	return handler, nil
}

func (cdi *CDIHandler) GetDevice(device string) *cdiapi.Device {
	return cdi.registry.DeviceDB().GetDevice(device)
}

func (cdi *CDIHandler) CreateCommonSpecFile() error {
	spec := &cdispec.Spec{
		Kind: cdiKind,
		Devices: []cdispec.Device{
			{
				Name: cdiCommonDeviceName,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("GPU_NODE_NAME=%s", os.Getenv("NODE_NAME")),
						fmt.Sprintf("DRA_RESOURCE_DRIVER_NAME=%s", DriverName),
					},
				},
			},
		},
	}

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %v", err)
	}
	spec.Version = minVersion

	specName, err := cdiapi.GenerateNameForTransientSpec(spec, cdiCommonDeviceName)
	if err != nil {
		return fmt.Errorf("failed to generate Spec name: %w", err)
	}

	return cdi.registry.SpecDB().WriteSpec(spec, specName)
}

func (cdi *CDIHandler) CreateClaimSpecFile(claimUid string, devices AllocatedDevices) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdiClass, claimUid)

	spec := &cdispec.Spec{
		Kind:    cdiKind,
		Devices: []cdispec.Device{},
	}

	gpuIdx := 0
	for _, device := range devices {
		switch device.Type() {
		case nascrd.GpuDeviceType:
			cdiDevice := cdispec.Device{
				Name: device.gpu.uuid,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("GPU_DEVICE_%d=%s", gpuIdx, device.gpu.uuid),
					},
				},
			}
			spec.Devices = append(spec.Devices, cdiDevice)
			gpuIdx++
		}
	}

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %v", err)
	}
	spec.Version = minVersion

	return cdi.registry.SpecDB().WriteSpec(spec, specName)
}

func (cdi *CDIHandler) DeleteClaimSpecFile(claimUid string) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdiClass, claimUid)
	return cdi.registry.SpecDB().RemoveSpec(specName)
}

func (cdi *CDIHandler) GetClaimDevices(claimUid string, devices AllocatedDevices) []string {
	cdiDevices := []string{
		cdiapi.QualifiedName(cdiVendor, cdiClass, cdiCommonDeviceName),
	}

	for _, device := range devices {
		switch device.Type() {
		case nascrd.GpuDeviceType:
			cdiDevice := cdiapi.QualifiedName(cdiVendor, cdiClass, device.gpu.uuid)
			cdiDevices = append(cdiDevices, cdiDevice)
		}
	}

	return cdiDevices
}