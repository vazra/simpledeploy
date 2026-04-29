package compose

import (
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

// helper: build an AppConfig with a single service for easy assertions.
func cfgWith(svc types.ServiceConfig) *AppConfig {
	return &AppConfig{
		Project: &types.Project{
			Services: types.Services{svc.Name: svc},
		},
	}
}

func TestValidate_Privileged(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", Privileged: true}))
	if len(v) == 0 {
		t.Fatal("expected violation for privileged")
	}
}

func TestValidate_NetworkHost(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", NetworkMode: "host"}))
	if len(v) == 0 {
		t.Fatal("expected violation for network_mode host")
	}
}

func TestValidate_DangerousCaps(t *testing.T) {
	cases := []string{"ALL", "SYS_ADMIN", "SYS_MODULE", "DAC_READ_SEARCH", "BPF", "NET_RAW", "CAP_SYS_ADMIN"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", CapAdd: []string{c}}))
			if len(v) == 0 {
				t.Fatalf("expected violation for cap %q", c)
			}
		})
	}
}

func TestValidate_DangerousSecurityOpt(t *testing.T) {
	cases := []string{"apparmor=unconfined", "seccomp=unconfined", "label=disable"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", SecurityOpt: []string{c}}))
			if len(v) == 0 {
				t.Fatalf("expected violation for security_opt %q", c)
			}
		})
	}
}

func TestValidate_UsernsHost(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", UserNSMode: "host"}))
	if len(v) == 0 {
		t.Fatal("expected violation for userns_mode host")
	}
}

func TestValidate_VolumesFromBlocked(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", VolumesFrom: []string{"other"}}))
	if len(v) == 0 {
		t.Fatal("expected violation for volumes_from")
	}
}

func TestValidate_PidContainer(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", Pid: "container:foo"}))
	if len(v) == 0 {
		t.Fatal("expected violation for pid container:")
	}
}

func TestValidate_DangerousBindMounts(t *testing.T) {
	cases := []string{"/var/lib/docker", "/boot", "/lib/modules", "/run", "/etc/passwd"}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{
				Name: "x",
				Volumes: []types.ServiceVolumeConfig{
					{Type: "bind", Source: src, Target: "/x"},
				},
			}))
			if len(v) == 0 {
				t.Fatalf("expected violation for bind %q", src)
			}
		})
	}
}

func TestValidate_RootBindMount(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{
		Name: "x",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "bind", Source: "/", Target: "/host"},
		},
	}))
	found := false
	for _, m := range v {
		if strings.Contains(m, "root filesystem") || strings.Contains(m, `"/"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected violation for / bind mount, got %v", v)
	}
}

func TestValidate_DriverOptsHostBindShim(t *testing.T) {
	cfg := &AppConfig{
		Project: &types.Project{
			Services: types.Services{
				"x": types.ServiceConfig{Name: "x"},
			},
			Volumes: types.Volumes{
				"shim": types.VolumeConfig{
					Driver: "local",
					DriverOpts: map[string]string{
						"type":   "none",
						"o":      "bind",
						"device": "/etc",
					},
				},
			},
		},
	}
	v := ValidateComposeSecurity(cfg)
	if len(v) == 0 {
		t.Fatal("expected violation for driver_opts host-bind shim into /etc")
	}
}

func TestValidate_AllowsInnocentService(t *testing.T) {
	v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{
		Name:  "web",
		Image: "nginx:alpine",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "bind", Source: "/var/lib/simpledeploy/apps/web/data", Target: "/data"},
		},
	}))
	if len(v) != 0 {
		t.Fatalf("expected zero violations for innocent service, got %v", v)
	}
}
