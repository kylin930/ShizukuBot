package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	Server struct {
		Addr string `json:"addr"`
	} `json:"server"`
	OSS struct {
		Region           string `json:"region"`
		Endpoint         string `json:"endpoint"`
		InternalEndpoint string `json:"internal_endpoint"`
		BucketName       string `json:"bucket_name"`
		PublicBase       string `json:"public_base"`
		STSAPI           string `json:"sts_api"`
	} `json:"oss"`
	Auth struct {
		LoginURL  string `json:"login_url"`
		Phone     string `json:"phone"`
		Password  string `json:"password"`
		CacheFile string `json:"cache_file"`
	} `json:"auth"`
}

var config Config

func InitConfig() error {
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		config.Server.Addr = ":8081"

		config.OSS.Region = "oss-cn-qingdao"
		config.OSS.Endpoint = "oss-cn-qingdao.aliyuncs.com"
		config.OSS.InternalEndpoint = "oss-cn-qingdao-internal.aliyuncs.com"
		config.OSS.BucketName = "vanthink"
		config.OSS.PublicBase = "https://vanthink.oss-cn-qingdao.aliyuncs.com"
		config.OSS.STSAPI = "https://api.wxzxzj.com/api/tool/upload/getStsToken"

		config.Auth.LoginURL = "https://api.wxzxzj.com/api/auth/student/login"
		config.Auth.CacheFile = "auth_cache.json"

		data, _ := json.MarshalIndent(config, "", "  ")
		os.WriteFile("config.json", data, 0644)
	}

	data, err := os.ReadFile("config.json")
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &config)
}
