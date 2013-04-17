package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	nonceSize           = 32
	oauthConsumerKey    = "Fnky4HZ8z4NsOxRniTvCA"
	oauthConsumerSecret = "iv9q7CTYfrls05eFlhyEkPpHcJqseSWpbDx8GIyGvg"
)

func GetTwitterRequestToken(callback string) (token string, token_secret string, callback_confirmed bool, err error) {

	client := &http.Client{}

	urlParams := make(map[string]string)

	authHeader, err := CreateAuthHeader("POST", "https://api.twitter.com/oauth/request_token", urlParams, callback, "", "")
	if err != nil {
		log.Printf("CreateAuthHeader resulted in an error: %s\n", err)
		return
	}

	req, err := http.NewRequest("POST", "https://api.twitter.com/oauth/request_token", nil)
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Authorization", authHeader)

	log.Println("Requesting token from Twitter")
	response, err := client.Do(req)

	if err != nil {
		log.Printf("Request resulted in an error: %s\n", err)
		return
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("Received an error while reading request token response: %s\n", err)
		return
	}

	dataValues, err := url.ParseQuery(string(contents))
	if err != nil {
		log.Printf("Received an error while parsing response from Twitter: %s\n", err)
		log.Printf("Response was: %s\n", contents)
		return
	}

	return dataValues["oauth_token"][0], dataValues["oauth_token_secret"][0], dataValues["oauth_token_secret"][0] == "true", nil
}

func GetTwitterAccessToken(token string, oauthVerifier string) (outToken string, tokenSecret string, userid string, screenName string, err error) {

	client := &http.Client{}

	urlParams := make(map[string]string)
	urlParams["oauth_verifier"] = oauthVerifier

	authHeader, err := CreateAuthHeader("POST", "https://api.twitter.com/oauth/access_token", urlParams, "", token, "")
	if err != nil {
		log.Printf("CreateAuthHeader resulted in an error: %s\n", err)
		return
	}

	body := strings.NewReader(fmt.Sprintf("oauth_verifier=%s", percentEncode(oauthVerifier)))

	req, err := http.NewRequest("POST", "https://api.twitter.com/oauth/access_token", body)
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Authorization", authHeader)

	log.Println("Requesting access token from Twitter")

	response, err := client.Do(req)

	if err != nil {
		log.Printf("Request resulted in an error: %s\n", err)
		return
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("Received an error while reading request token response: %s\n", err)
		return
	}

	log.Printf("Received: %s\n", contents)

	dataValues, err := url.ParseQuery(string(contents))
	if err != nil {
		log.Printf("Received an error while parsing response from Twitter: %s\n", err)
		log.Printf("Response was: %s\n", contents)
		return
	}

	oauthTokenVals, ok := dataValues["oauth_token"]
	if !ok {
		log.Print("No oauth_token returned\n")
	} else {
		outToken = oauthTokenVals[0]
	}

	oauthTokenSecretVals, ok := dataValues["oauth_token_secret"]
	if !ok {
		log.Print("No oauth_token_secret returned\n")
	} else {
		tokenSecret = oauthTokenSecretVals[0]
	}

	userIdVals, ok := dataValues["user_id"]
	if !ok {
		log.Print("No user_id returned\n")
	} else {
		userid = userIdVals[0]
	}

	screenNameVals, ok := dataValues["screen_name"]
	if !ok {
		log.Print("No screen_name returned\n")
	} else {
		screenName = screenNameVals[0]
	}

	return outToken, tokenSecret, userid, screenName, nil
}

func CreateAuthHeader(method string, url string, urlparams map[string]string, callback string, token string, tokenSecret string) (string, error) {

	// Generate a nonce
	//base64 encoding 32 bytes of random data

	nonce, err := RandomString(nonceSize)
	if err != nil {
		return "", err
	}

	log.Printf("Base64 encoded nonce: %s\n", nonce)

	// timestamp
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	// TEST OVERRIDE
	// nonce = "ea9ec8429b68d6b77cd5600adbbb0456"
	// timestamp = "1318467427"

	oauthParams := make(map[string]string)

	oauthParams["oauth_consumer_key"] = percentEncode(oauthConsumerKey)
	oauthParams["oauth_nonce"] = percentEncode(nonce)
	oauthParams["oauth_signature_method"] = "HMAC-SHA1"
	oauthParams["oauth_timestamp"] = percentEncode(timestamp)
	oauthParams["oauth_version"] = "1.0"

	if len(callback) > 0 {
		oauthParams["oauth_callback"] = percentEncode(callback)
	}
	if len(token) > 0 {
		oauthParams["oauth_token"] = percentEncode(token)
	}

	// Signature

	signature, err := SignRequest(method, url, urlparams, oauthParams, tokenSecret)
	if err != nil {
		return "", err
	}
	oauthParams["oauth_signature"] = percentEncode(signature)

	parts := make([]string, 0)

	keyList := make([]string, 0)
	for key, _ := range oauthParams {
		keyList = append(keyList, key)
	}
	sort.Strings(keyList)

	for _, key := range keyList {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, oauthParams[key]))
	}
	collectedParams := strings.Join(parts, ", ")

	authHeader := fmt.Sprintf("OAuth %s", collectedParams)

	return authHeader, nil
}

func SignRequest(method string, url string, urlparams map[string]string, oauthParams map[string]string, tokenSecret string) (string, error) {

	params := make(map[string]string)

	for key, value := range urlparams {
		if len(key) > 0 {
			params[percentEncode(key)] = percentEncode(value)
		}
	}

	for key, value := range oauthParams {
		if len(key) > 0 {
			params[key] = value
		}
	}

	parts := make([]string, 0)

	keyList := make([]string, 0)
	for key, _ := range params {
		keyList = append(keyList, key)
	}
	sort.Strings(keyList)

	for _, key := range keyList {
		parts = append(parts, fmt.Sprintf("%s=%s", key, params[key]))
	}
	collectedParams := strings.Join(parts, "&")
	log.Printf("collectedParams: %s\n", collectedParams)

	signatureBase := fmt.Sprintf("%s&%s&%s", strings.ToUpper(method), percentEncode(url), percentEncode(collectedParams))
	signingKey := fmt.Sprintf("%s&%s", percentEncode(oauthConsumerSecret), percentEncode(tokenSecret))

	// TEST OVERRIDE
	// signatureBase = "POST&https%3A%2F%2Fapi.twitter.com%2F1%2Fstatuses%2Fupdate.json&include_entities%3Dtrue%26oauth_consumer_key%3Dxvz1evFS4wEEPTGEFPHBog%26oauth_nonce%3DkYjzVBB8Y0ZFabxSWbWovY3uYSQ2pTgmZeNu2VS4cg%26oauth_signature_method%3DHMAC-SHA1%26oauth_timestamp%3D1318622958%26oauth_token%3D370773112-GmHxMAgYyLbNEtIKZeRNFsMKPR9EyMZeS9weJAEb%26oauth_version%3D1.0%26status%3DHello%2520Ladies%2520%252B%2520Gentlemen%252C%2520a%2520signed%2520OAuth%2520request%2521"
	// signingKey = "kAcSOqF21Fu85e7zjz7ZN2U4ZRhfV3WpwPAoE3Z7kBw&LswwdoUaIvS8ltyTt5jkRh4J50vUPVVHtR2YPi5kE"

	log.Printf("signatureBase: %s\n", signatureBase)
	log.Printf("signingKey: %s\n", signingKey)

	hasher := hmac.New(sha1.New, []byte(signingKey))
	n, err := hasher.Write([]byte(signatureBase))
	if n != len(signatureBase) || err != nil {
		log.Printf("Error hashing with hmac: %s\n", err)
		log.Printf("n: %d, len(signatureBase): %d\n", n, len(signatureBase))
		return "", err
	}
	shaHash := hasher.Sum(nil)

	sigBuffer := &bytes.Buffer{}
	sigEncoder := base64.NewEncoder(base64.StdEncoding, sigBuffer)
	sigEncoder.Write(shaHash)
	sigEncoder.Close()
	signature := sigBuffer.String()

	log.Printf("signature: %s\n", signature)

	return signature, err
}

func percentEncode(s string) string {
	hexCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			hexCount++
		}
	}
	if hexCount == 0 {
		return s // nothing to do
	}

	t := make([]byte, len(s)+2*hexCount)

	j := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		} else {
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

func shouldEscape(c byte) bool {
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
		return false
	}
	return true
}
