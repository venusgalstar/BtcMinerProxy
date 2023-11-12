package system

import (
	"syscall"
)

type LinuxConfigurator struct {
}

func NewOSConfigurator() *LinuxConfigurator {
	return &LinuxConfigurator{}
}

func (c *LinuxConfigurator) GetConfig() (*Config, error) {
	cfg := &Config{}
	localPortRange, err := sysctlGet("net.ipv4.ip_local_port_range")
	if err != nil {
		return nil, err
	}
	cfg.LocalPortRange = localPortRange

	tcpMaxSynBacklog, err := sysctlGet("net.ipv4.tcp_max_syn_backlog")
	if err != nil {
		return nil, err
	}
	cfg.TcpMaxSynBacklog = tcpMaxSynBacklog

	somaxconn, err := sysctlGet("net.core.somaxconn")
	if err != nil {
		return nil, err
	}
	cfg.Somaxconn = somaxconn

	netdevMaxBacklog, err := sysctlGet("net.core.netdev_max_backlog")
	if err != nil {
		return nil, err
	}
	cfg.NetdevMaxBacklog = netdevMaxBacklog

	var rlimit syscall.Rlimit
	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return nil, err
	}

	cfg.RlimitSoft = rlimit.Cur
	cfg.RlimitHard = rlimit.Max

	return cfg, nil
}

func (c *LinuxConfigurator) ApplyConfig(cfg *Config) error {
	err := sysctlSet("net.ipv4.ip_local_port_range", cfg.LocalPortRange)
	if err != nil {
		return err
	}
	err = sysctlSet("net.ipv4.tcp_max_syn_backlog", cfg.TcpMaxSynBacklog)
	if err != nil {
		return err
	}
	err = sysctlSet("net.core.somaxconn", cfg.Somaxconn)
	if err != nil {
		return err
	}
	err = sysctlSet("net.core.netdev_max_backlog", cfg.NetdevMaxBacklog)
	if err != nil {
		return err
	}
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{
		Cur: cfg.RlimitSoft,
		Max: cfg.RlimitHard,
	})
	return err
}
