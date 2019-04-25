// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
)

// IsPhysicalInterface returns true if the specified interface name is physical
func IsPhysicalInterface(ifname string) bool {
	fn := filepath.Join("/sys/class/net/", ifname)
	link, err := os.Readlink(fn)
	if err != nil {
		log.Infof("cannot read link %s", fn)
		return true
	}
	if strings.Contains(link, "/virtual/") {
		return false
	}
	return true
}

// initDistro gets distribution identification
// TODO: should we cache the initDistro? does it never change?
func initDistro() (distro string) {
	// Note: Order of checking is important because some distros share same file names
	// but with different function.
	// Keep this order: redhat based -> ubuntu -> debian

	// redhat
	if distro = utils.GetStrByKeyword(REDHAT, ""); distro != "" {
		return distro
	}
	// amazon linux
	distro = utils.GetStrByKeyword(AMAZON, "")

	ds := strings.Split(distro, ":")
	distro = ds[len(ds)-1]
	if distro != "" {
		distro = "Amzn Linux " + distro
		return distro
	}
	// ubuntu
	distro = utils.GetStrByKeyword(UBUNTU, "DISTRIB_DESCRIPTION")
	if distro != "" {
		ds = strings.Split(distro, "=")
		distro = ds[len(ds)-1]
		if distro != "" {
			distro = strings.Trim(distro, "\"")
		} else {
			distro = "Ubuntu unknown"
		}
		return distro
	}

	pathes := []string{DEBIAN, SUSE, SLACKWARE, GENTOO, OTHER}
	if path, line := utils.GetStrByKeywordFiles(pathes, ""); path != "" && line != "" {
		distro = line
		if path == DEBIAN {
			distro = "Debian " + distro
		}
		if idx := strings.Index(distro, "Alpine"); idx != -1 {
			distro = distro[idx:]
		}
	} else {
		distro = "Unknown"
	}
	return distro
}
