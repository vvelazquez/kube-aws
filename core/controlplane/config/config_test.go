package config

import (
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/kubernetes-incubator/kube-aws/model"
	"github.com/kubernetes-incubator/kube-aws/netutil"
)

const minimalConfigYaml = `externalDNSName: test.staging.core-os.net
keyName: test-key-name
region: us-west-1
clusterName: test-cluster-name
kmsKeyArn: "arn:aws:kms:us-west-1:xxxxxxxxx:key/xxxxxxxxxxxxxxxxxxx"
`

const minimalChinaConfigYaml = `externalDNSName: test.staging.core-os.net
keyName: test-key-name
region: cn-north-1
availabilityZone: cn-north-1a
clusterName: test-cluster-name
`

const availabilityZoneConfig = `
availabilityZone: us-west-1c
`

const singleAzConfigYaml = minimalConfigYaml + availabilityZoneConfig

var goodNetworkingConfigs = []string{
	``, //Tests validity of default network config values
	`
vpcCIDR: 10.4.3.0/24
instanceCIDR: 10.4.3.0/24
podCIDR: 172.4.0.0/16
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.5.100.101
`, `
vpcCIDR: 10.4.0.0/16
instanceCIDR: 10.4.3.0/24
podCIDR: 10.6.0.0/16
serviceCIDR: 10.5.0.0/16
dnsServiceIP: 10.5.100.101
`, `
vpcId: vpc-xxxxx
routeTableId: rtb-xxxxxx
`, `
vpcId: vpc-xxxxx
`, `
createRecordSet: false
hostedZoneId: ""
`, `
createRecordSet: true
recordSetTTL: 400
hostedZoneId: "XXXXXXXXXXX"
`, `
createRecordSet: true
hostedZoneId: "XXXXXXXXXXX"
`,
}

var incorrectNetworkingConfigs = []string{
	`
vpcCIDR: 10.4.2.0/23
instanceCIDR: 10.4.3.0/24
podCIDR: 10.4.0.0/16 #podCIDR contains vpcCDIR.
serviceCIDR: 10.5.0.0/16
dnsServiceIP: 10.5.100.101
`,
	`
vpcCIDR: 10.4.2.0/23
instanceCIDR: 10.4.3.0/24
podCIDR: 10.5.0.0/16
serviceCIDR: 10.4.0.0/16 #serviceCIDR contains vpcCDIR.
dnsServiceIP: 10.4.100.101
`, `
vpcCIDR: 10.4.0.0/16
instanceCIDR: 10.5.3.0/24 #instanceCIDR not in vpcCIDR
podCIDR: 10.6.0.0/16
serviceCIDR: 10.5.0.0/16
dnsServiceIP: 10.5.100.101
`, `
vpcCIDR: 10.4.3.0/16
instanceCIDR: 10.4.3.0/24
podCIDR: 172.4.0.0/16
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.5.0.1 #dnsServiceIP conflicts with kubernetesServiceIP
`, `
vpcCIDR: 10.4.3.0/16
instanceCIDR: 10.4.3.0/24
podCIDR: 10.4.0.0/16 #vpcCIDR overlaps with podCIDR
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.5.100.101

`, `
vpcCIDR: 10.4.3.0/16
instanceCIDR: 10.4.3.0/24
podCIDR: 172.4.0.0/16
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.6.100.101 #dnsServiceIP not in service CIDR
`, `
routeTableId: rtb-xxxxxx # routeTableId specified without vpcId
`, `
# invalid TTL
recordSetTTL: 0
`, `
# hostedZone and hostedZoneID shouldn't be blank when createRecordSet is true
createRecordSet: true
`, `
# recordSetTTL shouldn't be modified when createRecordSet is false
createRecordSet: false
recordSetTTL: 400
`, `
# hostedZoneId should'nt be modified when createRecordSet is false
createRecordSet: false
hostedZoneId: /hostedzone/staging_id_2 #hostedZone and hostedZoneId defined
`, `
# hostedZone had been deprecated and then dropped
createRecordSet: true
hostedZone: "staging.core-os.net"
`,
}

func TestNetworkValidation(t *testing.T) {

	for _, networkConfig := range goodNetworkingConfigs {
		configBody := singleAzConfigYaml + networkConfig
		if _, err := ClusterFromBytes([]byte(configBody)); err != nil {
			t.Errorf("Correct config tested invalid: %s\n%s", err, networkConfig)
		}
	}

	for _, networkConfig := range incorrectNetworkingConfigs {
		configBody := singleAzConfigYaml + networkConfig
		if _, err := ClusterFromBytes([]byte(configBody)); err == nil {
			t.Errorf("Incorrect config tested valid, expected error:\n%s", networkConfig)
		}
	}

}

func TestMinimalChinaConfig(t *testing.T) {
	c, err := ClusterFromBytes([]byte(minimalChinaConfigYaml))
	if err != nil {
		t.Errorf("Failed to parse config %s: %v", minimalChinaConfigYaml, err)
	}

	if !c.Region.IsChina() {
		t.Error("IsChinaRegion test failed.")
	}

	if c.AssetsEncryptionEnabled() {
		t.Error("Assets encryption must be disabled on China.")
	}
}

func TestKubernetesServiceIPInference(t *testing.T) {

	// We sill assert that after parsing the network configuration,
	// KubernetesServiceIP is the correct pre-determined value
	testConfigs := []struct {
		NetworkConfig       string
		KubernetesServiceIP string
	}{
		{
			NetworkConfig: `
serviceCIDR: 172.5.10.10/22
dnsServiceIP: 172.5.10.10
        `,
			KubernetesServiceIP: "172.5.8.1",
		},
		{
			NetworkConfig: `
serviceCIDR: 10.5.70.10/18
dnsServiceIP: 10.5.64.10
        `,
			KubernetesServiceIP: "10.5.64.1",
		},
		{
			NetworkConfig: `
serviceCIDR: 172.4.155.98/27
dnsServiceIP: 172.4.155.100
        `,
			KubernetesServiceIP: "172.4.155.97",
		},
		{
			NetworkConfig: `
serviceCIDR: 10.6.142.100/28
dnsServiceIP: 10.6.142.100
        `,
			KubernetesServiceIP: "10.6.142.97",
		},
	}

	for _, testConfig := range testConfigs {
		configBody := singleAzConfigYaml + testConfig.NetworkConfig
		cluster, err := ClusterFromBytes([]byte(configBody))
		if err != nil {
			t.Errorf("Unexpected error parsing config: %v\n %s", err, configBody)
			continue
		}

		_, serviceNet, err := net.ParseCIDR(cluster.ServiceCIDR)
		if err != nil {
			t.Errorf("invalid serviceCIDR: %v", err)
			continue
		}

		kubernetesServiceIP := netutil.IncrementIP(serviceNet.IP)
		if kubernetesServiceIP.String() != testConfig.KubernetesServiceIP {
			t.Errorf("KubernetesServiceIP mismatch: got %s, expected %s",
				kubernetesServiceIP,
				testConfig.KubernetesServiceIP)
		}
	}

}

func TestReleaseChannel(t *testing.T) {

	validConfigs := []struct {
		conf    string
		channel string
	}{
		{
			conf: `
releaseChannel: alpha
`,
			channel: "alpha",
		},
		{
			conf: `
releaseChannel: beta
`,
			channel: "beta",
		},
		{
			conf: `
releaseChannel: stable
`,
			channel: "stable",
		},
	}

	invalidConfigs := []string{
		`
releaseChannel: non-existant #this release channel will never exist
`,
	}

	for _, conf := range validConfigs {
		confBody := singleAzConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if c.ReleaseChannel != conf.channel {
			t.Errorf(
				"parsed release channel %s does not match config: %s",
				c.ReleaseChannel,
				confBody,
			)
		}
	}

	for _, conf := range invalidConfigs {
		confBody := singleAzConfigYaml + conf
		_, err := ClusterFromBytes([]byte(confBody))
		if err == nil {
			t.Errorf("expected error parsing invalid config: %s", confBody)
		}
	}

}

func TestAvailabilityZones(t *testing.T) {
	testCases := []struct {
		conf string
		azs  []string
	}{
		{
			conf: singleAzConfigYaml,
			azs:  []string{"us-west-1c"},
		},
		{
			conf: minimalConfigYaml + `
# You can specify multiple subnets to be created in order to achieve H/A
vpcCIDR: 10.4.3.0/16
subnets:
  - availabilityZone: ap-northeast-1a
    instanceCIDR: 10.4.3.0/24
  - availabilityZone: ap-northeast-1c
    instanceCIDR: 10.4.4.0/24
`,
			azs: []string{"ap-northeast-1a", "ap-northeast-1c"},
		},
	}

	for _, conf := range testCases {
		confBody := conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}

		actualAzs := c.AvailabilityZones()
		if !reflect.DeepEqual(actualAzs, conf.azs) {
			t.Errorf(
				"availability zones %s do not match actual list %s in config: %s",
				conf.azs,
				actualAzs,
				confBody,
			)
		}
	}
}

func TestMultipleSubnets(t *testing.T) {

	validConfigs := []struct {
		conf    string
		subnets []model.Subnet
	}{
		{
			conf: `
# You can specify multiple subnets to be created in order to achieve H/A
vpcCIDR: 10.4.3.0/16
subnets:
  - availabilityZone: ap-northeast-1a
    instanceCIDR: 10.4.3.0/24
  - availabilityZone: ap-northeast-1c
    instanceCIDR: 10.4.4.0/24
`,
			subnets: []model.Subnet{
				{
					InstanceCIDR:     "10.4.3.0/24",
					AvailabilityZone: "ap-northeast-1a",
					Name:             "Subnet0",
				},
				{
					InstanceCIDR:     "10.4.4.0/24",
					AvailabilityZone: "ap-northeast-1c",
					Name:             "Subnet1",
				},
			},
		},
		{
			conf: `
# Given AZ/CIDR, missing subnets fall-back to the single subnet with the AZ/CIDR given.
vpcCIDR: 10.4.3.0/16
availabilityZone: ap-northeast-1a
instanceCIDR: 10.4.3.0/24
`,
			subnets: []model.Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.4.3.0/24",
					Name:             "Subnet0",
				},
			},
		},
		{
			conf: `
# Given AZ/CIDR, empty subnets fall-back to the single subnet with the AZ/CIDR given.
vpcCIDR: 10.4.3.0/16
availabilityZone: ap-northeast-1a
instanceCIDR: 10.4.3.0/24
subnets: []
`,
			subnets: []model.Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.4.3.0/24",
					Name:             "Subnet0",
				},
			},
		},
		{
			conf: `
# Given no AZ/CIDR, empty subnets fall-backs to the single subnet with the default az/cidr.
availabilityZone: "ap-northeast-1a"
subnets: []
`,
			subnets: []model.Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.0.0.0/24",
					Name:             "Subnet0",
				},
			},
		},
		{
			conf: `
# Missing subnets field fall-backs to the single subnet with the default az/cidr.
availabilityZone: "ap-northeast-1a"
`,
			subnets: []model.Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.0.0.0/24",
					Name:             "Subnet0",
				},
			},
		},
	}

	invalidConfigs := []string{
		`
# You can't specify both the top-level availability zone and subnets
# (It doesn't make sense. Which configuration did you want, single or multi AZ one?)
availabilityZone: "ap-northeast-1a"
subnets:
  - availabilityZone: "ap-northeast-1b"
    instanceCIDR: "10.0.0.0/24"
`,
		`
# You can't specify both the top-level instanceCIDR and subnets
# (It doesn't make sense. Which configuration did you want, single or multi AZ one?)
instanceCIDR: "10.0.0.0/24"
subnets:
- availabilityZone: "ap-northeast-1b"
  instanceCIDR: "10.0.1.0/24"
`,
		`
subnets:
# Missing AZ like this
# - availabilityZone: "ap-northeast-1a"
- instanceCIDR: 10.0.0.0/24
`,
		`
subnets:
# Missing AZ like this
# - availabilityZone: "ap-northeast-1a"
- instanceCIDR: 10.0.0.0/24
`,
		`
subnets:
# Overlapping subnets
- availabilityZone: "ap-northeast-1a"
  instanceCIDR: 10.0.5.0/24
- availabilityZone: "ap-northeast-1b"
  instanceCIDR: 10.0.5.0/24
`,
	}

	for _, conf := range validConfigs {
		confBody := minimalConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if !reflect.DeepEqual(c.Subnets, conf.subnets) {
			t.Errorf(
				"parsed subnets %+v does not match expected subnets %+v in config: %s",
				c.Subnets,
				conf.subnets,
				confBody,
			)
		}
	}

	for _, conf := range invalidConfigs {
		confBody := minimalConfigYaml + conf
		_, err := ClusterFromBytes([]byte(confBody))
		if err == nil {
			t.Errorf("expected error parsing invalid config:\n%s", confBody)
		}
	}

}

func TestControllerVolumeType(t *testing.T) {

	validConfigs := []struct {
		conf       string
		volumeType string
		iops       int
	}{
		{
			conf:       ``,
			volumeType: "gp2",
			iops:       0,
		},
		{
			conf: `
controller:
  rootVolume:
    type: gp2
`,
			volumeType: "gp2",
			iops:       0,
		},
		{
			conf: `
controller:
  rootVolume:
    type: standard
`,
			volumeType: "standard",
			iops:       0,
		},
		{
			conf: `
controller:
  rootVolume:
    type: io1
    iops: 100
`,
			volumeType: "io1",
			iops:       100,
		},
		{
			conf: `
controller:
  rootVolume:
    type: io1
    iops: 2000
`,
			volumeType: "io1",
			iops:       2000,
		},
		// TODO Remove test cases for deprecated keys in v0.9.7
		{
			conf: `
controllerRootVolumeType: gp2
`,
			volumeType: "gp2",
			iops:       0,
		},
		{
			conf: `
controllerRootVolumeType: standard
`,
			volumeType: "standard",
			iops:       0,
		},
		{
			conf: `
controllerRootVolumeType: io1
controllerRootVolumeIOPS: 100
`,
			volumeType: "io1",
			iops:       100,
		},
		{
			conf: `
controllerRootVolumeType: io1
controllerRootVolumeIOPS: 2000
`,
			volumeType: "io1",
			iops:       2000,
		},
	}

	invalidConfigs := []string{
		`
# There's no volume type 'default'
controllerRootVolumeType: default
`,
		`
# IOPS must be zero for volume types != 'io1'
controllerRootVolumeType: standard
controllerRootVolumeIOPS: 100
`,
		`
# IOPS must be zero for volume types != 'io1'
controllerRootVolumeType: gp2
controllerRootVolumeIOPS: 2000
`,
		`
# IOPS smaller than the minimum (100)
controllerRootVolumeType: io1
controllerRootVolumeIOPS: 99
`,
		`
# IOPS greater than the maximum (2000)
controllerRootVolumeType: io1
controllerRootVolumeIOPS: 2001
`,
	}

	for _, conf := range validConfigs {
		confBody := singleAzConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if c.Controller.RootVolume.Type != conf.volumeType {
			t.Errorf(
				"parsed root volume type %s does not match root volume %s in config: %s",
				c.Controller.RootVolume.Type,
				conf.volumeType,
				confBody,
			)
		}
	}

	for _, conf := range invalidConfigs {
		confBody := singleAzConfigYaml + conf
		_, err := ClusterFromBytes([]byte(confBody))
		if err == nil {
			t.Errorf("expected error parsing invalid config: %s", confBody)
		}
	}
}

func TestWorkerVolumeType(t *testing.T) {

	validConfigs := []struct {
		conf       string
		volumeType string
		iops       int
	}{
		{
			conf:       ``,
			volumeType: "gp2",
			iops:       0,
		},
		{
			conf: `
workerRootVolumeType: gp2
`,
			volumeType: "gp2",
			iops:       0,
		},
		{
			conf: `
workerRootVolumeType: standard
`,
			volumeType: "standard",
			iops:       0,
		},
		{
			conf: `
workerRootVolumeType: io1
workerRootVolumeIOPS: 100
`,
			volumeType: "io1",
			iops:       100,
		},
		{
			conf: `
workerRootVolumeType: io1
workerRootVolumeIOPS: 2000
`,
			volumeType: "io1",
			iops:       2000,
		},
	}

	invalidConfigs := []string{
		`
# There's no volume type 'default'
workerRootVolumeType: default
`,
		`
# IOPS must be zero for volume types != 'io1'
workerRootVolumeType: standard
workerRootVolumeIOPS: 100
`,
		`
# IOPS must be zero for volume types != 'io1'
workerRootVolumeType: gp2
workerRootVolumeIOPS: 2000
`,
		`
# IOPS smaller than the minimum (100)
workerRootVolumeType: io1
workerRootVolumeIOPS: 99
`,
		`
# IOPS greater than the maximum (2000)
workerRootVolumeType: io1
workerRootVolumeIOPS: 2001
`,
	}

	for _, conf := range validConfigs {
		confBody := singleAzConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if c.WorkerRootVolumeType != conf.volumeType {
			t.Errorf(
				"parsed root volume type %s does not match root volume %s in config: %s",
				c.WorkerRootVolumeType,
				conf.volumeType,
				confBody,
			)
		}
	}

	for _, conf := range invalidConfigs {
		confBody := singleAzConfigYaml + conf
		_, err := ClusterFromBytes([]byte(confBody))
		if err == nil {
			t.Errorf("expected error parsing invalid config: %s", confBody)
		}
	}
}

func TestNodeDrainerConfig(t *testing.T) {

	validConfigs := []struct {
		conf        string
		nodeDrainer NodeDrainer
	}{
		{
			conf: `
`,
			nodeDrainer: NodeDrainer{
				Enabled: false,
			},
		},
		{
			conf: `
experimental:
  nodeDrainer:
    enabled: false
`,
			nodeDrainer: NodeDrainer{
				Enabled: false,
			},
		},
		{
			conf: `
experimental:
  nodeDrainer:
    enabled: true
`,
			nodeDrainer: NodeDrainer{
				Enabled: true,
			},
		},
	}

	for _, conf := range validConfigs {
		confBody := singleAzConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if !reflect.DeepEqual(c.Experimental.NodeDrainer, conf.nodeDrainer) {
			t.Errorf(
				"parsed node drainer settings %+v does not match config: %s",
				c.Experimental.NodeDrainer,
				confBody,
			)
		}
	}

}

func TestTLSBootstrapConfig(t *testing.T) {

	validConfigs := []struct {
		conf         string
		tlsBootstrap TLSBootstrap
	}{
		{
			conf: `
`,
			tlsBootstrap: TLSBootstrap{
				Enabled: false,
			},
		},
		{
			conf: `
experimental:
  tlsBootstrap:
    enabled: false
`,
			tlsBootstrap: TLSBootstrap{
				Enabled: false,
			},
		},
		{
			conf: `
experimental:
  tlsBootstrap:
    enabled: true
`,
			tlsBootstrap: TLSBootstrap{
				Enabled: true,
			},
		},
		{
			conf: `
# Settings for an experimental feature must be under the "experimental" field. Ignored.
tlsBootstrap:
  enabled: true
`,
			tlsBootstrap: TLSBootstrap{
				Enabled: false,
			},
		},
	}

	for _, conf := range validConfigs {
		confBody := singleAzConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if !reflect.DeepEqual(c.Experimental.TLSBootstrap, conf.tlsBootstrap) {
			t.Errorf(
				"parsed TLS bootstrap settings %+v does not match config: %s",
				c.Experimental.TLSBootstrap,
				confBody,
			)
		}
	}

}

func TestRktConfig(t *testing.T) {
	validChannels := []string{
		"alpha",
		"beta",
		"stable",
	}

	conf := func(channel string) string {
		return fmt.Sprintf(`containerRuntime: rkt
releaseChannel: %s
`, channel)
	}

	for _, channel := range validChannels {
		confBody := singleAzConfigYaml + conf(channel)
		cluster, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
		}

		_, err2 := cluster.Config()
		if err2 != nil {
			t.Errorf("failed to generate config for %s: %v", channel, err2)
		}
	}
}

func TestValidateExistingVPC(t *testing.T) {
	validCases := []struct {
		vpc     string
		subnets []string
	}{
		{"10.0.0.0/16", []string{"10.0.3.0/24", "10.0.4.0/24"}},
	}

	invalidCases := []struct {
		vpc     string
		subnets []string
	}{
		// both subnets conflicts
		{"10.0.0.0/16", []string{"10.0.1.0/24", "10.0.2.0/24"}},
		// 10.0.1.0/24 conflicts
		{"10.0.0.0/16", []string{"10.0.1.0/24", "10.0.3.0/24"}},
		// 10.0.2.0/24 conflicts
		{"10.0.0.0/16", []string{"10.0.2.0/24", "10.0.3.0/24"}},
		// vpc cidr doesn't match
		{"10.1.0.0/16", []string{"10.1.1.0/24", "10.1.2.0/24"}},
		// vpc cidr is invalid
		{"1o.1.o.o/16", []string{"10.1.1.0/24", "10.1.2.0/24"}},
		// subnet cidr is invalid
		{"10.1.0.0/16", []string{"1o.1.1.o/24", "10.1.2.0/24"}},
	}

	cluster := NewDefaultCluster()

	cluster.VPCCIDR = "10.0.0.0/16"
	cluster.Subnets = []model.Subnet{
		model.NewPublicSubnet("ap-northeast-1a", "10.0.1.0/24"),
		model.NewPublicSubnet("ap-northeast-1a", "10.0.2.0/24"),
	}

	for _, testCase := range validCases {
		err := cluster.ValidateExistingVPC(testCase.vpc, testCase.subnets)

		if err != nil {
			t.Errorf("failed to validate existing vpc and subnets: %v", err)
		}
	}

	for _, testCase := range invalidCases {
		err := cluster.ValidateExistingVPC(testCase.vpc, testCase.subnets)

		if err == nil {
			t.Errorf("expected to fail validating existing vpc and subnets: %v", testCase)
		}
	}
}

func TestWithTrailingDot(t *testing.T) {
	tests := [][]string{
		[]string{
			"",
			"",
		},
		[]string{
			"foo.bar.",
			"foo.bar.",
		},
		[]string{
			"foo.bar",
			"foo.bar.",
		},
	}

	for _, test := range tests {
		input := test[0]
		actual := WithTrailingDot(input)
		expected := test[1]

		if expected != actual {
			t.Errorf(
				"WithTrailingDot(\"%s\") expected to return \"%s\" but it returned: \"%s\"",
				input,
				expected,
				actual,
			)
		}
	}
}
