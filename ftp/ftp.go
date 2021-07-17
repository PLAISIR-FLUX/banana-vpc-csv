package ftp

import (
	"errors"
	"fmt"
	"net"
	"regexp"
)

func Dial(hosts ...string) (*Client, error) {
	return DialConfig(Config{}, hosts...)
}

func DialConfig(config Config, hosts ...string) (*Client, error) {
	expandedHosts, err := lookupHosts(hosts, config.IPv6Lookup)
	if err != nil {
		return nil, err
	}
	return newClient(config, expandedHosts), nil
}

var hasPort = regexp.MustCompile(`^[^:]+:\d+$|\]:\d+$`)

func lookupHosts(hosts []string, ipv6Lookup bool) ([]string, error) {
	if len(hosts) == 0 {
		return nil, errors.New("must specify at least one host")
	}
	var (
		ret  []string
		ipv6 []string
	)
	for i, host := range hosts {
		if !hasPort.MatchString(host) {
			host = fmt.Sprintf("[%s]:21", host)
		}
		hostnameOrIP, port, err := net.SplitHostPort(host)
		if err != nil {
			return nil, fmt.Errorf(`invalid host "%s"`, hosts[i])
		}
		if net.ParseIP(hostnameOrIP) != nil {
			ret = append(ret, host)
		} else {
			ips, err := net.LookupIP(hostnameOrIP)
			if err != nil {
				return nil, fmt.Errorf(`error resolving host "%s": %s`, hostnameOrIP, err)
			}
			for _, ip := range ips {
				ipAndPort := fmt.Sprintf("[%s]:%s", ip.String(), port)
				if ip.To4() == nil && !ipv6Lookup {
					ipv6 = append(ipv6, ipAndPort)
				} else {
					ret = append(ret, ipAndPort)
				}
			}
		}
	}
	if len(ret) == 0 && len(ipv6) > 0 {
		return ipv6, nil
	}
	return ret, nil
}
