// Copyright 2017-2023 The Cloudprober Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	configpb "github.com/cloudprober/cloudprober/config/proto"
	"github.com/cloudprober/cloudprober/logger"
	probespb "github.com/cloudprober/cloudprober/probes/proto"
	surfacerspb "github.com/cloudprober/cloudprober/surfacers/proto"
	targetspb "github.com/cloudprober/cloudprober/targets/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func testUnmarshalConfig(t *testing.T, fileName string) (*configpb.ProberConfig, error) {
	t.Helper()

	configStr, configFormat, err := readConfigFile(fileName)
	if err != nil {
		t.Error(err)
	}
	return unmarshalConfig(configStr, configFormat)
}

func TestUnmarshalConfig(t *testing.T) {
	wantCfg := &configpb.ProberConfig{
		Probe: []*probespb.ProbeDef{
			{
				Name: proto.String("dns_k8s"),
				Type: probespb.ProbeDef_DNS.Enum(),
				Targets: &targetspb.TargetsDef{
					Type: &targetspb.TargetsDef_HostNames{
						HostNames: "10.0.0.1",
					},
				},
			},
		},
		Surfacer: []*surfacerspb.SurfacerDef{
			{
				Type: surfacerspb.Type_STACKDRIVER.Enum(),
			},
		},
	}
	tests := []struct {
		name           string
		configFile     string
		baseConfigFile string
		want           *configpb.ProberConfig
		wantErr        bool
	}{
		{
			name:       "textpb",
			configFile: "testdata/cloudprober_base.cfg",
			want:       wantCfg,
		},
		{
			name:           "yaml",
			configFile:     "testdata/cloudprober.yaml",
			baseConfigFile: "testdata/cloudprober.cfg",
		},
		{
			name:           "json",
			configFile:     "testdata/cloudprober.json",
			baseConfigFile: "testdata/cloudprober.cfg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testUnmarshalConfig(t, tt.configFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigToProto() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.want == nil {
				cfg, err := testUnmarshalConfig(t, tt.baseConfigFile)
				if err != nil {
					t.Errorf("Error reading the base config itself: %v", err)
				}
				tt.want = cfg
			}
			assert.Equal(t, tt.want.String(), got.String())
		})
	}
}

func TestConfigTest(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		cs         ConfigSource
		wantErr    bool
	}{
		{
			name:       "valid_base",
			configFile: "testdata/cloudprober_base.cfg",
		},
		{
			name:       "invalid_without_vars",
			configFile: "testdata/cloudprober_invalid.cfg",
			wantErr:    true,
		},
		{
			name:    "no_config_error",
			wantErr: true,
		},
		{
			name: "valid_with_vars",
			cs: &defaultConfigSource{
				BaseVars: map[string]string{
					"probetype": "HTTP",
				},
			},
			configFile: "testdata/cloudprober_invalid.cfg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*configFile = tt.configFile
			if err := ConfigTest(tt.cs); (err != nil) != tt.wantErr {
				t.Errorf("ConfigTest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDumpConfig(t *testing.T) {
	tests := []struct {
		configFile string
		format     string
		want       string
		wantErr    bool
	}{
		{
			configFile: "testdata/cloudprober_base.cfg",
			format:     "yaml",
			want: `
probe:
- name: dns_k8s
  targets:
    hostNames: 10.0.0.1
  type: DNS
surfacer:
- type: STACKDRIVER
`,
		},
		{

			configFile: "testdata/cloudprober_base.cfg",
			format:     "json",
			want: `
{
	"probe": [{
			"name": "dns_k8s",
			"type": "DNS",
			"targets": {"hostNames": "10.0.0.1"}
	}],
    "surfacer": [{
		"type": "STACKDRIVER"
	}]
}`,
		},
		{

			configFile: "testdata/cloudprober_base.cfg",
			format:     "textpb",
			want: `
probe: {
  name: "dns_k8s"
  type: DNS
  targets: {
    host_names: "10.0.0.1"
  }
}
surfacer: {
  type: STACKDRIVER
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, err := DumpConfig(tt.format, &defaultConfigSource{FileName: tt.configFile})
			if (err != nil) != tt.wantErr {
				t.Errorf("DumpConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			switch tt.format {
			case "json":
				var g interface{}
				var w interface{}
				assert.NoError(t, json.Unmarshal(got, &g))
				assert.NoError(t, json.Unmarshal([]byte(tt.want), &w))
				assert.Equal(t, w, g)
			case "textpb":
				var g configpb.ProberConfig
				var w configpb.ProberConfig
				assert.NoError(t, prototext.Unmarshal(got, &g))
				assert.NoError(t, prototext.Unmarshal([]byte(tt.want), &w))
				assert.Equal(t, w.String(), g.String())
			default:
				tt.want = strings.TrimLeft(tt.want, "\n")
				assert.Equal(t, tt.want, string(got))
			}
		})
	}
}

func TestSubstEnvVars(t *testing.T) {
	os.Setenv("SECRET_PROBE_NAME1", "testprobe")
	os.Setenv("SECRET_PROBE_NAME2", "x")
	os.Setenv("SECRET_PROBE_TYPE", "SECRET")
	// Make sure this env var is not set, for error behavior testing.
	os.Unsetenv("SECRET_PROBEX_NAME")

	tests := []struct {
		name      string
		configStr string
		want      string
		wantLog   string
	}{
		{
			name:      "no_env_vars",
			configStr: `probe {name: "dns_k8s"}`,
			want:      `probe {name: "dns_k8s"}`,
		},
		{
			name:      "env_var",
			configStr: `probe {name: "**$SECRET_PROBE_NAME2**"}`,
			want:      `probe {name: "x"}`,
		},
		{
			name:      "env_var_concat",
			configStr: `probe {name: "**$SECRET_PROBE_NAME1**-**$SECRET_PROBE_NAME2**"}`,
			want:      `probe {name: "testprobe-x"}`,
		},
		{
			name:      "env_var_partial",
			configStr: `probe {name: "**$SECRET_PROBE_NAME1**-**$PASSWORD**"}`,
			want:      `probe {name: "testprobe-**$PASSWORD**"}`,
		},
		{
			name: "env_var_multi_line",
			configStr: `probe {
				name: "**$SECRET_PROBE_NAME1**"
				type: "**$SECRET_PROBE_TYPE**"
			}`,
			want: `probe {
				name: "testprobe"
				type: "SECRET"
			}`,
		},
		{
			name:      "env_var_not_defined",
			configStr: `probe {name: "**$SECRET_PROBEX_NAME**"}`,
			want:      `probe {name: "**$SECRET_PROBEX_NAME**"}`,
			wantLog:   "SECRET_PROBEX_NAME not defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := logger.New(logger.WithWriter(&buf))
			assert.Equal(t, tt.want, substEnvVars(tt.configStr, l))
			assert.Contains(t, buf.String(), tt.wantLog)
		})
	}
}
