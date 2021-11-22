package core

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/ptypes"
	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vagrant-plugin-sdk/component"
	"github.com/hashicorp/vagrant-plugin-sdk/core"
	"github.com/hashicorp/vagrant-plugin-sdk/proto/vagrant_plugin_sdk"
	"github.com/hashicorp/vagrant/internal/server/proto/vagrant_server"
)

type Machine struct {
	*Target
	box     *Box
	machine *vagrant_server.Target_Machine
	logger  hclog.Logger
	guest   core.Guest
}

// Close implements core.Machine
func (m *Machine) Close() (err error) {
	return
}

// ID implements core.Machine
func (m *Machine) ID() (id string, err error) {
	return m.machine.Id, nil
}

// SetID implements core.Machine
func (m *Machine) SetID(value string) (err error) {
	m.machine.Id = value
	return m.SaveMachine()
}

func (m *Machine) Box() (b core.Box, err error) {
	if m.box == nil {
		// TODO: get provider info here too/generate full machine config?
		// We know that these are machines so, save the Machine record
		boxes, _ := m.project.Boxes()
		b, err := boxes.Find(m.target.Configuration.ConfigVm.Box, "")
		if err != nil {
			return nil, err
		}
		if b == nil {
			// Add the box
			b, err = addBox(m.target.Configuration.ConfigVm.Box, "virtualbox", m.project.basis)
			if err != nil {
				return nil, err
			}
		}
		m.machine.Box = b.(*Box).ToProto()
		m.Save()
		m.box = b.(*Box)
	}

	return m.box, nil
}

// Guest implements core.Machine
func (m *Machine) Guest() (g core.Guest, err error) {
	if m.guest != nil {
		return m.guest, nil
	}
	guests, err := m.project.basis.typeComponents(m.ctx, component.GuestType)
	if err != nil {
		return
	}

	var result core.Guest
	var result_name string
	var numParents int

	for name, g := range guests {
		guest := g.Value.(core.Guest)
		detected, err := guest.Detect(m.toTarget())
		if err != nil {
			m.logger.Error("guest error on detection check",
				"plugin", name,
				"type", "Guest",
				"error", err)

			continue
		}
		if result == nil {
			if detected {
				result = guest
				result_name = name
				if numParents, err = m.project.basis.countParents(guest); err != nil {
					return nil, err
				}
			}
			continue
		}

		if detected {
			gp, err := m.project.basis.countParents(guest)
			if err != nil {
				m.logger.Error("failed to get parents from guest",
					"plugin", name,
					"type", "Guest",
					"error", err,
				)

				continue
			}

			if gp > numParents {
				result = guest
				result_name = name
				numParents = gp
			}
		}
	}

	if result == nil {
		return nil, fmt.Errorf("failed to detect guest plugin for current platform")
	}

	m.logger.Info("guest detection complete",
		"name", result_name)

	if s, ok := result.(core.Seeder); ok {
		p, _ := m.Project()
		if err = s.Seed(m, p, m.Target); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("guest plugin does not support seeder interface")
	}

	m.guest = result

	return result, nil
}

func (m *Machine) Inspect() (printable string, err error) {
	name, err := m.Name()
	provider, err := m.Provider()
	printable = "#<" + reflect.TypeOf(m).String() + ": " + name + " (" + reflect.TypeOf(provider).String() + ")>"
	return
}

// Reload implements core.Machine
func (m *Machine) Reload() (err error) {
	// TODO
	return
}

// ConnectionInfo implements core.Machine
func (m *Machine) ConnectionInfo() (info *core.ConnectionInfo, err error) {
	// TODO: need Vagrantfile
	return
}

// MachineState implements core.Machine
func (m *Machine) MachineState() (state *core.MachineState, err error) {
	var result core.MachineState
	return &result, mapstructure.Decode(m.machine.State, &result)
}

// SetMachineState implements core.Machine
func (m *Machine) SetMachineState(state *core.MachineState) (err error) {
	var st *vagrant_plugin_sdk.Args_Target_Machine_State
	mapstructure.Decode(state, &st)
	m.machine.State = st
	return m.SaveMachine()
}

func (m *Machine) UID() (userId string, err error) {
	return m.machine.Uid, nil
}

// SyncedFolders implements core.Machine
func (m *Machine) SyncedFolders() (folders []core.SyncedFolder, err error) {
	config := m.target.Configuration
	machineConfig := config.ConfigVm
	syncedFolders := machineConfig.SyncedFolders

	folders = []core.SyncedFolder{}
	for _, folder := range syncedFolders {
		// TODO: get default synced folder type
		folder.Type = "virtualbox"
		plg, err := m.project.basis.component(m.ctx, component.SyncedFolderType, folder.Type)
		// TODO: configure with folder info
		if err != nil {
			return nil, err
		}
		folders = append(folders, plg.Value.(core.SyncedFolder))
	}
	return
}

func (m *Machine) SaveMachine() (err error) {
	m.logger.Debug("saving machine to db", "machine", m.machine.Id)
	m.target.Record, err = ptypes.MarshalAny(m.machine)
	if err != nil {
		return nil
	}
	return m.Save()
}

func (m *Machine) toTarget() core.Target {
	return m
}

var _ core.Machine = (*Machine)(nil)
var _ core.Target = (*Machine)(nil)
