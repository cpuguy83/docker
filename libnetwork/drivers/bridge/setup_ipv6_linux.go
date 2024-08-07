package bridge

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	"github.com/containerd/log"
)

// Standard link local prefix
var linkLocalPrefix = netip.MustParsePrefix("fe80::/64")

const (
	ipv6ForwardConfPerm    = 0o644
	ipv6ForwardConfDefault = "/proc/sys/net/ipv6/conf/default/forwarding"
	ipv6ForwardConfAll     = "/proc/sys/net/ipv6/conf/all/forwarding"
)

func setupBridgeIPv6(config *networkConfiguration, i *bridgeInterface) error {
	procFile := "/proc/sys/net/ipv6/conf/" + config.BridgeName + "/disable_ipv6"
	ipv6BridgeData, err := os.ReadFile(procFile)
	if err != nil {
		return fmt.Errorf("Cannot read IPv6 setup for bridge %v: %v", config.BridgeName, err)
	}
	// Enable IPv6 on the bridge only if it isn't already enabled
	if ipv6BridgeData[0] != '0' {
		if err := os.WriteFile(procFile, []byte{'0', '\n'}, ipv6ForwardConfPerm); err != nil {
			return fmt.Errorf("Unable to enable IPv6 addresses on bridge: %v", err)
		}
	}

	// Remove unwanted addresses from the bridge, add required addresses, and assign
	// values to "i.bridgeIPv6", "i.gatewayIPv6".
	if err := i.programIPv6Addresses(config); err != nil {
		return err
	}
	return nil
}

func setupGatewayIPv6(config *networkConfiguration, i *bridgeInterface) error {
	if !config.AddressIPv6.Contains(config.DefaultGatewayIPv6) {
		return &ErrInvalidGateway{}
	}

	// Store requested default gateway
	i.gatewayIPv6 = config.DefaultGatewayIPv6

	return nil
}

func setupIPv6Forwarding(config *networkConfiguration, i *bridgeInterface) error {
	// Get current IPv6 default forwarding setup
	ipv6ForwardDataDefault, err := os.ReadFile(ipv6ForwardConfDefault)
	if err != nil {
		return fmt.Errorf("Cannot read IPv6 default forwarding setup: %v", err)
	}
	// Enable IPv6 default forwarding only if it is not already enabled
	if ipv6ForwardDataDefault[0] != '1' {
		if err := os.WriteFile(ipv6ForwardConfDefault, []byte{'1', '\n'}, ipv6ForwardConfPerm); err != nil {
			log.G(context.TODO()).Warnf("Unable to enable IPv6 default forwarding: %v", err)
		}
	}

	// Get current IPv6 all forwarding setup
	ipv6ForwardDataAll, err := os.ReadFile(ipv6ForwardConfAll)
	if err != nil {
		return fmt.Errorf("Cannot read IPv6 all forwarding setup: %v", err)
	}
	// Enable IPv6 all forwarding only if it is not already enabled
	if ipv6ForwardDataAll[0] != '1' {
		if err := os.WriteFile(ipv6ForwardConfAll, []byte{'1', '\n'}, ipv6ForwardConfPerm); err != nil {
			log.G(context.TODO()).Warnf("Unable to enable IPv6 all forwarding: %v", err)
		}
	}

	return nil
}
