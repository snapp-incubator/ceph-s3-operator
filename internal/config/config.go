package config

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	koanf "github.com/knadh/koanf/v2"
)

type Rgw struct {
	Endpoint  string `koanf:"endpoint"`
	AccessKey string `koanf:"accessKey"`
	SecretKey string `koanf:"secretKey"`
}

type Config struct {
	S3UserClass                     string `koanf:"s3UserClass"`
	ClusterName                     string `koanf:"clusterName"`
	ValidationWebhookTimeoutSeconds int    `koanf:"validationWebhookTimeoutSeconds"`
	Rgw                             *Rgw   `koanf:"rgw"`
}

var (
	defaultConfig = Config{
		S3UserClass:                     "ceph-default",
		ClusterName:                     "okd4-main",
		ValidationWebhookTimeoutSeconds: 10,
		Rgw: &Rgw{
			Endpoint:  "http://localhost:8000",
			AccessKey: "2262XNX11FZRR44XWIRD",
			SecretKey: "rmtuS1Uj1bIC08QFYGW18GfSHAbkPqdsuYynNudw",
		},
	}
)

func GetConfig(configPath string) (*Config, error) {
	k := koanf.New(".")
	parser := yaml.Parser()
	cfg := &Config{}

	if err := k.Load(structs.Provider(defaultConfig, "koanf"), nil); err != nil {
		return nil, err
	}

	if err := k.Load(file.Provider(configPath), parser); err != nil {
		return nil, err
	}

	if err := k.Unmarshal("", cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
