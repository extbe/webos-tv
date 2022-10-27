package webostv

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

const (
	multicastAddress = "239.255.255.250"
	multicastPort    = 1900
	mx               = 2

	timeoutSeconds = 5

	msgTemplate = "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: {{.multicastAddress}}:{{.multicastPort}}\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"ST: {{.st}}\r\n" +
		"MX: {{.mx}}\r\n" +
		"\r\n"

	locationHeaderPrefix = "Location: "
)

var (
	discoveryMsgTemplate  *template.Template
	multicastAddressBytes [4]byte
)

func init() {
	tpl, err := template.New("discovery-message").Parse(msgTemplate)
	if err != nil {
		panic(err)
	}

	discoveryMsgTemplate = tpl

	for i, octetStr := range strings.Split(multicastAddress, ".") {
		octet, err := strconv.Atoi(octetStr)
		if err != nil {
			panic(err)
		}

		multicastAddressBytes[i] = byte(octet)
	}
}

func discover(service string, keyword string) ([]string, error) {
	var msg bytes.Buffer
	err := discoveryMsgTemplate.Execute(&msg, map[string]interface{}{
		"multicastAddress": multicastAddress,
		"multicastPort":    multicastPort,
		"st":               service,
		"mx":               mx,
	})
	if err != nil {
		return nil, err
	}

	fd, err := prepareDiscoverySocket()
	if err != nil {
		return nil, err
	}

	dstAddr := &syscall.SockaddrInet4{
		Port: multicastPort,
		Addr: multicastAddressBytes,
	}
	err = syscall.Sendto(fd, msg.Bytes(), 0, dstAddr)
	if err != nil {
		return nil, err
	}

	locations := make(map[string]struct{})

	for {
		buf := make([]byte, 1024)
		_, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			errno, ok := err.(syscall.Errno)
			if ok && errno.Timeout() {
				break
			} else {
				return nil, err
			}
		}

		location := parseLocation(buf)

		isLocationValid, err := validateDevice(location, keyword)
		if err != nil {
			return nil, err
		}

		if isLocationValid {
			locations[location] = struct{}{}
		}
	}

	return getMapKeys(locations), nil
}

func prepareDiscoverySocket() (int, error) {
	// ForkLock docs state that socket syscall requires the lock
	syscall.ForkLock.Lock()

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return 0, err
	}

	syscall.ForkLock.Unlock()

	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		_ = syscall.Close(fd)
		return 0, err
	}

	if err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 2); err != nil {
		_ = syscall.Close(fd)
		return 0, err
	}

	timeVal := new(syscall.Timeval)
	timeVal.Sec = timeoutSeconds
	if err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, timeVal); err != nil {
		_ = syscall.Close(fd)
		return 0, err
	}

	return fd, nil
}

func parseLocation(buf []byte) string {
	rsp := string(buf)
	for _, line := range strings.Split(rsp, "\r\n") {
		if strings.HasPrefix(line, locationHeaderPrefix) {
			return line[len(locationHeaderPrefix):]
		}
	}

	return ""
}

func validateDevice(location string, keyword string) (bool, error) {
	response, err := http.Get(location)
	if err != nil {
		return false, err
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return false, err
	}

	return strings.Contains(string(body), keyword), nil
}

func getMapKeys(locations map[string]struct{}) []string {
	deviceLocations := make([]string, len(locations))
	i := 0
	for location := range locations {
		deviceLocations[i] = location
		i++
	}
	return deviceLocations
}
