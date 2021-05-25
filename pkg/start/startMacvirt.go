package start

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	cidataISO, err := cidata.GenerateISO9660(instName, y)
	if err != nil {
		return err
	}
	if err := iso9660util.Write(filepath.Join(instDir, "cidata.iso"), cidataISO); err != nil {
		return err
	}

	//use macvirt
	mvCfg := macvirt.Config{
		Name:        instName,
		InstanceDir: instDir,
		LimaYAML:    y,
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
	defer func() {
		_ = mvirtCmd.Process.Kill()
	}()

	sshFixCmd := exec.Command("ssh-keygen",
		"-R", fmt.Sprintf("[127.0.0.1]:%d", y.SSH.LocalPort),
		"-R", fmt.Sprintf("[localhost]:%d", y.SSH.LocalPort),
	)

	if out, err := sshFixCmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to run %v: %q", sshFixCmd.Args, string(out))
	}
	logrus.Infof("SSH: 127.0.0.1:%d", y.SSH.LocalPort)

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
