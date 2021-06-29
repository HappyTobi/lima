package macvirt

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"net"
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
	"gopkg.in/yaml.v2"
)

type Config struct {
	Name        string
	InstanceDir string
	LimaYAML    *limayaml.LimaYAML
	VmState     State
}

type State struct {
	MacAddress string
	IpAddress  string
}

const SateFileName = "state.yml"

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
	args = append(args, fmt.Sprintf("--mac-address=%s", cfg.VmState.MacAddress))
	args = append(args, fmt.Sprintf("--kernel-path=%s", filepath.Join(cfg.InstanceDir, "vmlinuz")))
	args = append(args, fmt.Sprintf("--initrd-path=%s", filepath.Join(cfg.InstanceDir, "initrd")))
	args = append(args, fmt.Sprintf("--disk-path=%s", filepath.Join(cfg.InstanceDir, "basedisk")))
	args = append(args, fmt.Sprintf("--cloud-init-data-path=%s", filepath.Join(cfg.InstanceDir, "cidata.iso")))
	args = append(args, fmt.Sprintf("--cmd-line-arg=%s", cfg.LimaYAML.Cmdline))

	return exe, args, nil
}

func FetchIpAddress(macAddress string) (string, error) {
	arpData, err := exec.Command("arp", "-an").Output()
	if err != nil {
		return "", err
	}

	for _, row := range strings.Split(string(arpData), "\n") {
		fields := strings.Fields(row)
		if len(fields) > 3 {
			ipR := []rune(fields[1]) //ips
			arpMac := strings.Split(fields[3], ":")
			if len(arpMac) > 5 {
				formattedMac := fmt.Sprintf("%02s:%02s:%02s:%02s:%02s:%02s", arpMac[0], arpMac[1], arpMac[2], arpMac[3], arpMac[4], arpMac[5])
				if formattedMac == macAddress {
					return string(ipR[1 : len(fields[1])-1]), nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not find ip for passed macAddress %s", macAddress)
}

func LoadVmState(stateFilePath string) (State, error) {
	//generate state file
	var state State
	if _, err := os.Stat(stateFilePath); errors.Is(err, os.ErrNotExist) {
		macAddr := generateMac()
		state.MacAddress = macAddr
		//save state because next vm start needs same mac
		err = saveStateFile(state, stateFilePath)
		if err != nil {
			return state, err
		}
		return state, nil
	}

	return loadStateFile(stateFilePath)
}

func UpdateVmState(state State, stateFilePath string) error {
	return saveStateFile(state, stateFilePath)
}

func loadStateFile(stateFilePath string) (State, error) {
	var state State
	data, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		return state, err
	}

	if err := yaml.Unmarshal(data, &state); err != nil {
		return state, err
	}

	return state, nil
}

func saveStateFile(state State, stateFilePath string) error {
	data, err := yaml.Marshal(&state)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(stateFilePath, data, 0660)
}

//https://stackoverflow.com/questions/21018729/generate-mac-address-in-go
func generateMac() string {
	buf := make([]byte, 6)
	rand.Read(buf)

	// Set the local bit and unicast address
	buf[0] = (buf[0] | 2) & 0xfe

	var mac net.HardwareAddr
	mac = append(mac, buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
	return mac.String()
}
