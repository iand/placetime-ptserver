package main

type TwitterUser struct {
	Name                 string `json:"name"`
	Location             string `json:"location"`
	Description          string `json:"description"`
	Url                  string `json:"url"`
	Verified             bool   `json:"verified"`
	Lang                 string `json:"lang"`
	ProfileImageUrl      string `json:"profile_image_url"`
	ProfileImageUrlHttps string `json:"profile_image_url_https"`
	ProfileBannerUrl     string `json:"profile_banner_url"`
	DefaultProfile       bool   `json:"default_profile"`
	DefaultProfileImage  bool   `json:"default_profile_image"`
}
