package macvirt

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AkihiroSuda/lima/pkg/limayaml"
	"github.com/AkihiroSuda/lima/pkg/localpathutil"
	"github.com/docker/go-units"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Config struct {
	Name        string
	InstanceDir string
	LimaYAML    *limayaml.LimaYAML
}

func checkVmFiles(vmFiles []string) bool {
	for _, vmFile := range vmFiles {
		if _, err := os.Stat(vmFile); errors.Is(err, os.ErrNotExist) {
			return true
		}
	}
	return false
}

func EnsureDisk(cfg Config) error {
	diffDisk := filepath.Join(cfg.InstanceDir, "diffdisk")
	if _, err := os.Stat(diffDisk); err == nil || !errors.Is(err, os.ErrNotExist) {
		// disk is already ensured
		return err
	}

	baseDisk := filepath.Join(cfg.InstanceDir, "basedisk")
	vmlinuz := filepath.Join(cfg.InstanceDir, "vmlinuz")
	initrd := filepath.Join(cfg.InstanceDir, "initrd")
	vmFiles := []string{baseDisk, vmlinuz, initrd}

	if checkVmFiles(vmFiles) {
		downloadTmp := filepath.Join(cfg.InstanceDir, "download.tmp")
		if err := os.RemoveAll(downloadTmp); err != nil {
			return err
		}
		var ensuredBaseFiles bool
		errs := make([]error, len(cfg.LimaYAML.Images))
		for i, f := range cfg.LimaYAML.Images {
			if f.Arch != cfg.LimaYAML.Arch {
				errs[i] = fmt.Errorf("unsupported arch: %q", f.Arch)
				continue
			}
			url := f.Location
			if !strings.Contains(url, "://") {
				expanded, err := localpathutil.Expand(url)
				if err != nil {
					return err
				}
				url = "file://" + expanded
			}

			cmd := exec.Command("curl", "-fSL", "-o", downloadTmp, url)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			logrus.Infof("Attempting to download the image from %q", url)
			if err := cmd.Run(); err != nil {
				errs[i] = errors.Wrapf(err, "failed to run %v", cmd.Args)
				continue
			}

			fileName := filepath.Join(cfg.InstanceDir, f.Name)
			if _, err := os.Stat(downloadTmp); err == nil && strings.Contains(url, "tar.gz") {
				ok, err := extractImageArchive(cfg.InstanceDir, downloadTmp)
				if err != nil {
					return err
				}
				if ok {
					logrus.Infof("Downloaded file extracted %s -> %s", url, downloadTmp)
				}
			}
			//check type, if
			if err := os.Rename(downloadTmp, fileName); err != nil {
				return err
			}

			//only go ahead when all required files are downloaded
			if checkVmFiles(vmFiles) {
				ensuredBaseFiles = true
			}
		}
		if !ensuredBaseFiles {
			return errors.Errorf("failed to download the image, attempted %d candidates, errors=%v",
				len(cfg.LimaYAML.Images), errs)
		}
	}
	diskSize, err := units.RAMInBytes(cfg.LimaYAML.Disk)
	if err != nil {
		return err
	}
	diskSize = diskSize / 1024 / 1024
	cmd := exec.Command("dd", "if=/dev/null", fmt.Sprintf("of=%s", baseDisk), "bs=1m", "count=0", fmt.Sprintf("seek=%s", strconv.Itoa(int(diskSize))))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to run %v: %q", cmd.Args, string(out))
	}
	return nil
}

func extractImageArchive(directory string, filePath string) (bool, error) {
	downloadTmpExtract := filepath.Join(directory, "download.tmp.extract")
	downloadFile, err := os.Open(filePath)
	if err != nil {
		return false, errors.Wrapf(err, "failed to open file %v", downloadFile)
	}

	gzipStream, err := gzip.NewReader(downloadFile)
	defer gzipStream.Close()

	if err != nil {
		return false, errors.Wrapf(err, "failed to open file gzip stream %v", downloadFile)
	}

	tarStream := tar.NewReader(gzipStream)

	//rename extracted file and delete original one
	defer func(orgFile string, extFile string) {
		os.Remove(orgFile)
		os.Rename(extFile, orgFile)
	}(filePath, downloadTmpExtract)

	for {
		header, err := tarStream.Next()
		switch {
		case err == io.EOF:
			return true, nil
		case err != nil:
			return false, errors.Wrapf(err, "failed walk through tar file %v", downloadFile)
		}

		//only handle file
		f, err := os.OpenFile(downloadTmpExtract, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
		if err != nil {
			return false, err
		}

		// copy over contents
		if _, err := io.Copy(f, tarStream); err != nil {
			return false, err
		}
	}
}

func Cmdline(cfg Config) (string, []string, error) {
	exeBase := "macvirt"
	exe, err := exec.LookPath(exeBase)
	if err != nil {
		return "", nil, err
	}

	var args []string
	args = append(args, "--memory=1024") //0cfg.LimaYAML.Memory)
	args = append(args, fmt.Sprintf("--cpu-count=%s", strconv.Itoa(cfg.LimaYAML.CPUs)))
	args = append(args, fmt.Sprintf("--kernel-path=%s", filepath.Join(cfg.InstanceDir, "vmlinuz")))
	args = append(args, fmt.Sprintf("--initrd-path=%s", filepath.Join(cfg.InstanceDir, "initrd")))
	args = append(args, fmt.Sprintf("--disk-path=%s", filepath.Join(cfg.InstanceDir, "basedisk")))
	args = append(args, fmt.Sprintf("--cloud-init-data-path=%s", filepath.Join(cfg.InstanceDir, "cidata.iso")))
	args = append(args, fmt.Sprintf("--cmd-line-arg=%s", cfg.LimaYAML.Cmdline))

	return exe, args, nil
}

/*
func Cmdline(cfg Config) (string, []string, error) {
	y := cfg.LimaYAML
	exeBase := "qemu-system-" + y.Arch
	exe, err := exec.LookPath(exeBase)
	if err != nil {
		return "", nil, err
	}
	var args []string

	// Architecture
	accel := getAccel(y.Arch)
	switch y.Arch {
	case limayaml.X8664:
		// NOTE: "-cpu host" seems to cause kernel panic
		// (MacBookPro 2020, Intel(R) Core(TM) i7-1068NG7 CPU @ 2.30GHz, macOS 11.3, Ubuntu 21.04)
		args = append(args, "-cpu", "Haswell-v4")
		args = append(args, "-machine", "q35,accel="+accel)
	case limayaml.AARCH64:
		args = append(args, "-cpu", "cortex-a72")
		args = append(args, "-machine", "virt,accel="+accel)
	}

	// SMP
	args = append(args, "-smp",
		fmt.Sprintf("%d,sockets=1,cores=%d,threads=1", y.CPUs, y.CPUs))

	// Memory
	memBytes, err := units.RAMInBytes(y.Memory)
	if err != nil {
		return "", nil, err
	}
	args = append(args, "-m", strconv.Itoa(int(memBytes>>20)))

	// Firmware
	if !y.Firmware.LegacyBIOS {
		firmware := filepath.Join(exe,
			fmt.Sprintf("../../share/qemu/edk2-%s-code.fd", y.Arch))
		if _, err := os.Stat(firmware); err != nil {
			return "", nil, err
		}
		args = append(args, "-drive", fmt.Sprintf("if=pflash,format=raw,readonly,file=%s", firmware))
	} else if y.Arch != limayaml.X8664 {
		logrus.Warnf("field `firmware.legacyBIOS` is not supported for architecture %q, ignoring", y.Arch)
	}

	// Root disk
	args = append(args, "-drive", fmt.Sprintf("file=%s,if=virtio", filepath.Join(cfg.InstanceDir, "diffdisk")))
	args = append(args, "-boot", "c")

	// cloud-init
	args = append(args, "-cdrom", filepath.Join(cfg.InstanceDir, "cidata.iso"))

	// Network
	// CIDR is intentionally hardcoded to 192.168.5.0/24, as each of QEMU has its own independent slirp network.
	// TODO: enable bridge (with sudo?)
	args = append(args, "-net", "nic,model=virtio")
	args = append(args, "-net", fmt.Sprintf("user,net=192.168.5.0/24,hostfwd=tcp:127.0.0.1:%d-:22", y.SSH.LocalPort))

	// virtio-rng-pci acceralates starting up the OS, according to https://wiki.gentoo.org/wiki/QEMU/Options
	args = append(args, "-device", "virtio-rng-pci")

	// Misc devices
	switch y.Arch {
	case limayaml.X8664:
		args = append(args, "-device", "virtio-vga")
		args = append(args, "-device", "virtio-keyboard-pci")
		args = append(args, "-device", "virtio-mouse-pci")
	default:
		// QEMU does not seem to support virtio-vga for aarch64
		args = append(args, "-vga", "none", "-device", "ramfb")
		args = append(args, "-device", "usb-ehci")
		args = append(args, "-device", "usb-kbd")
		args = append(args, "-device", "usb-mouse")
	}
	args = append(args, "-parallel", "none")

	// We also want to enable vsock and virtfs here, but QEMU does not support vsock and virtfs for macOS hosts

	// QEMU process
	args = append(args, "-name", "lima-"+cfg.Name)
	args = append(args, "-pidfile", filepath.Join(cfg.InstanceDir, "qemu-pid"))

	return exe, args, nil
}

func getAccel(arch limayaml.Arch) string {
	nativeX8664 := arch == limayaml.X8664 && runtime.GOARCH == "amd64"
	nativeAARCH64 := arch == limayaml.AARCH64 && runtime.GOARCH == "arm64"
	native := nativeX8664 || nativeAARCH64
	if native {
		switch runtime.GOOS {
		case "darwin":
			return "hvf"
		case "linux":
			return "kvm"
		}
	}
	return "tcg"
}
*/
