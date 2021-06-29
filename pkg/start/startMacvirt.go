package start

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AkihiroSuda/lima/pkg/cidata"
	"github.com/AkihiroSuda/lima/pkg/hostagent"
	"github.com/AkihiroSuda/lima/pkg/iso9660util"
	"github.com/AkihiroSuda/lima/pkg/limayaml"
	"github.com/AkihiroSuda/lima/pkg/macvirt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type macvirtEmulator struct {
	ctx  context.Context
	name string
}

func NewMacVirtEmulator(ctx context.Context) Emulator {
	return &macvirtEmulator{ctx: ctx, name: "macvirt"}
}

func (e *macvirtEmulator) Start(instName string, instDir string, y *limayaml.LimaYAML) error {
	//add emulator to yml.
	y.Emulator = e.name

	cidataISO, err := cidata.GenerateISO9660(instName, y)
	if err != nil {
		return err
	}
	if err := iso9660util.Write(filepath.Join(instDir, "cidata.iso"), cidataISO); err != nil {
		return err
	}

	//check state configuration for macvirt
	state, err := macvirt.LoadVmState(filepath.Join(instDir, macvirt.SateFileName))
	if err != nil {
		return err
	}

	//use macvirt
	mvCfg := macvirt.Config{
		Name:        instName,
		InstanceDir: instDir,
		LimaYAML:    y,
		VmState:     state,
	}

	if err := macvirt.EnsureDisk(mvCfg); err != nil {
		return err
	}

	exe, macvirtArgs, err := macvirt.Cmdline(mvCfg)
	logrus.Debugf("Arguments: %s", strings.Join(macvirtArgs, " "))
	if err != nil {
		return err
	}
	mvirtCmd := exec.CommandContext(e.ctx, exe, macvirtArgs...)
	mvirtCmd.Stdout = os.Stdout
	mvirtCmd.Stderr = os.Stderr
	//mvirtCmd.Stdin =
	logrus.Info("Starting Macvirt initramfs boot")
	if err := mvirtCmd.Start(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var ip string
	for {
		ip, _ = macvirt.FetchIpAddress(mvCfg.VmState.MacAddress)
		if len(ip) > 0 {
			break
		}
		if ctx.Err() != nil {
			return err
		}
		time.Sleep(1 * time.Second)
	}
	mvCfg.VmState.IpAddress = ip

	if ok := macvirt.UpdateVmState(mvCfg.VmState, filepath.Join(instDir, macvirt.SateFileName)); ok != nil {
		logrus.WithError(err).Warn(("Could not save vm state file"))
	}

	defer func() {
		_ = mvirtCmd.Process.Kill()
	}()

	//get ip adress to use
	sshFixCmd := exec.Command("ssh-keygen",
		"-R", fmt.Sprintf("[%s]:%d", mvCfg.VmState.IpAddress, y.SSH.LocalPort),
		"-R", fmt.Sprintf("[%s]:%d", mvCfg.VmState.IpAddress, y.SSH.LocalPort),
	)

	if out, err := sshFixCmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to run %v: %q", sshFixCmd.Args, string(out))
	}
	logrus.Infof("SSH: %s:%d", mvCfg.VmState.IpAddress, y.SSH.LocalPort)

	hAgent, err := hostagent.New(y, instDir)
	if err != nil {
		return err
	}
	defer func() {
		if cErr := hAgent.Close(); cErr != nil {
			logrus.WithError(cErr).Warn("An error during shutting down the host agent")
		}
	}()
	if err := hAgent.Run(e.ctx); err == nil {
		logrus.Info("READY. Run `lima bash` to open the shell.")
	} else {
		logrus.WithError(err).Warn("DEGRADED. The VM seems running, but file sharing and port forwarding may not work.")
	}
	// TODO: daemonize the host process here
	return mvirtCmd.Wait()
}

func (e *macvirtEmulator) EmulatorName() string {
	return e.name
}
