package macvirt

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestMacGenerator(t *testing.T) {
	mac := generateMac()
	fmt.Println(mac)
	t.Log(string(mac))
}

func TestFetchIpAddress(t *testing.T) {
	_, err := FetchIpAddress("")
	assert.ErrorContains(t, err, "could not find ip for passed macAddress")
}

func TestFooBar(t *testing.T) {

	formattedMac, _ := net.ParseMAC("ee:c7:65:ce:78:02")
	formattedMac2, _ := net.ParseMAC("ee:c7:65:ce:78:2")
	max := strings.Split("ee:c7:65:ce:78:2", ":")
	mac := fmt.Sprintf("%02s:%02s:%02s:%02s:%02s:%02s", max[0], max[1], max[2], max[3], max[4], max[5])

	fmt.Println(formattedMac)
	fmt.Println(mac)
	fmt.Println(formattedMac2)
	t.Log(string(formattedMac))
}
