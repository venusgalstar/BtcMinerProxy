package system

import (
	"strings"
	"syscall"
)

type DarwinConfigurator struct {
}

func NewOSConfigurator() *DarwinConfigurator {
	return &DarwinConfigurator{}
}

func (c *DarwinConfigurator) GetConfig() (*Config, error) {
	cfg := &Config{}
	portRangeFirst, err := sysctlGet("net.inet.ip.portrange.first")
	if err != nil {
		return nil, err
	}
	portRangeLast, err := sysctlGet("net.inet.ip.portrange.last")
	if err != nil {
		return nil, err
	}
	localPortRange := portRangeFirst + " " + portRangeLast
	cfg.LocalPortRange = localPortRange

	// net.ipv4.tcp_max_syn_backlog is not available on Darwin

	somaxconn, err := sysctlGet("kern.ipc.somaxconn")
	if err != nil {
		return nil, err
	}
	cfg.Somaxconn = somaxconn

	// net.core.netdev_max_backlog is not available on Darwin

	var rlimit syscall.Rlimit
	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return nil, err
	}

	cfg.RlimitSoft = rlimit.Cur
	cfg.RlimitHard = rlimit.Max

	return cfg, nil
}

func (c *DarwinConfigurator) ApplyConfig(cfg *Config) error {
	rng := strings.Split(cfg.LocalPortRange, " ")
	err := sysctlSet("net.inet.ip.portrange.first", rng[0])
	if err != nil {
		return err
	}
	err = sysctlSet("net.inet.ip.portrange.last", rng[1])
	if err != nil {
		return err
	}

	// net.ipv4.tcp_max_syn_backlog is not available on Darwin

	err = sysctlSet("kern.ipc.somaxconn", cfg.Somaxconn)
	if err != nil {
		return err
	}

	// net.core.netdev_max_backlog is not available on Darwin

	// TODO: ensure these limits are actually applied
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{
		Cur: cfg.RlimitSoft,
		Max: cfg.RlimitHard,
	})
	return err
}
