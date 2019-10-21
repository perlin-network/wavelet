package main

import (
	"fmt"
	"github.com/perlin-network/wavelet/sys"
	"time"
)

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
)

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

var updateDirectory = func() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		/*
		 * XXX:TODO: Handle this error better
		 */
		return "/"
	}

	return path.Join(cacheDir, "wavelet")
}()

func validateSignature(publicKeyPEM []byte, signatureHex string, hash []byte) error {
	/*
	 * The public key is PEM encoded and must be decoded
	 * into a DER encoded byte stream.
	 */
	publicKeyDER, _ := pem.Decode(publicKeyPEM)
	if publicKeyDER == nil {
		return errors.New("Unable to decode PEM-encoded public key")
	}

	/*
	 * Parse the DER encoded public key into the RSA PublicKey
	 * structure (N, E)
	 */
	publicKey, err := x509.ParsePKIXPublicKey(publicKeyDER.Bytes)
	if err != nil {
		return err
	}

	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return err
	}

	switch publicKey := publicKey.(type) {
	case *rsa.PublicKey:
		err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
		return err
	}

	return errors.New("Unable to validate signature")
}

/* Fetch Latest Version Number */
func checkUpdateVersion(baseURL string) uint32 {
	latestVersionURL := fmt.Sprintf("%s/version", baseURL)

	handle, err := http.Get(latestVersionURL)
	if err != nil {
		return 0
	}
	defer handle.Body.Close()

	response, err := ioutil.ReadAll(handle.Body)
	if err != nil {
		return 0
	}

	latestVersionString := strings.TrimRight(string(response), "\r\n")
	latestVersion, err := strconv.ParseUint(latestVersionString, 16, 32)
	if err != nil {
		return 0
	}

	return uint32(latestVersion)
}

func downloadUpdate(baseURL string, updateDirectory string, publicKeyPEM []byte) error {
	/*
	 * Compute the URLs to the resources we will fetch
	 */
	binaryURL := fmt.Sprintf("%s/wavelet", baseURL)
	signatureURL := fmt.Sprintf("%s/wavelet.signature", baseURL)

	/*
	 * Create the update directory
	 */
	os.MkdirAll(updateDirectory, 0755)

	/*
	 * Download the signature, which is a hex encoded
	 * string containing the PKCS#1 v1.5 signature of
	 * the new binary.
	 */
	signatureHandle, err := http.Get(signatureURL)
	if err != nil {
		return fmt.Errorf("Unable to download signature: %+v", err)
	}
	defer signatureHandle.Body.Close()

	signatureHexBuffer, err := ioutil.ReadAll(signatureHandle.Body)
	if err != nil {
		return fmt.Errorf("Unable to read signature: %+v", err)
	}

	signatureHex := strings.TrimRight(string(signatureHexBuffer), "\r\n")

	/*
	 * Download the new binary to a temporary
	 * location while we validate the signature.
	 */
	binaryFileName := path.Join(updateDirectory, "wavelet")
	binaryTempFileName := path.Join(updateDirectory, "wavelet.new")
	binaryOldFileName  := path.Join(updateDirectory, "wavelet.old")
	binaryTempFile, err := os.OpenFile(binaryTempFileName, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("Unable to create temporary file: %+v", err)
	}
	defer binaryTempFile.Close()
	defer os.Remove(binaryTempFileName)

	binaryHandle, err := http.Get(binaryURL)
	if err != nil {
		return fmt.Errorf("Unable to download update file (begin): %+v", err)
	}

	_, err = io.Copy(binaryTempFile, binaryHandle.Body)
	if err != nil {
		return fmt.Errorf("Unable to download update file (body): %+v", err)
	}

	/*
	 * Create a hash of the download contents
	 */
	_, err = binaryTempFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("Unable to access updated file: %+v", err)
	}

	hasher := sha256.New()
	_, err = io.Copy(hasher, binaryTempFile)
	if err != nil {
		return fmt.Errorf("Unable to hash update file: %+v", err)
	}

	binarySHA256 := hasher.Sum(nil)

	/*
	 * Validate the signature
	 */
	validSignatureError := validateSignature(publicKeyPEM, signatureHex, binarySHA256)
	if validSignatureError != nil {
		return fmt.Errorf("Unable to verify downloaded signature: %+v", validSignatureError)
	}

	/*
	 * Rename the binary into place
	 */
	err = binaryTempFile.Close()
	if err != nil {
		return fmt.Errorf("Unable to flush file contents to disk: %+v", err)
	}

	err = os.Chmod(binaryTempFileName, 0700)
	if err != nil {
		return fmt.Errorf("Unable set mode on old file: %+v", err)
	}

	/*
	 * Remove any spurious copy of the old binary and rename the
	 * existing binary out of the way.  This is significant on
	 * Windows because we cannot delete the running binary
	 * while it is running.  We do not check for any errors here
	 * since most error cases do not matter, the error cases that
	 * do matter will be detected at a later step
	 */
	os.Remove(binaryOldFileName)
	os.Rename(binaryFileName, binaryOldFileName)

	err = os.Rename(binaryTempFileName, binaryFileName)
	if err != nil {
		/*
		 * If there was an error renaming things into
		 * their final location, attempt to put things
		 * back to the way they were before we tried
		 */
		os.Remove(binaryFileName)
		os.Rename(binaryOldFileName, binaryFileName)

		return fmt.Errorf("Unable rename file into place: %+v", err)
	}

	os.Remove(binaryOldFileName)

	return nil
}

func checkForUpdate(baseURL string, updateDirectory string, publicKeyPEM []byte, currentVersion uint32) {
	/* Check for a newer version binary */
	remoteVersion := checkUpdateVersion(baseURL)

	/*
	 * If that version number is lower than our version,
	 * do nothing.  To prevent down-grade attacks we
	 * will not attempt to switch to an older version
	 */
	if remoteVersion <= currentVersion {
		return
	}

	/*
	 * A newer version has been found, proceed with
	 * the replacement
	 *
	 * Download the new version and the signature
	 */
	downloadError := downloadUpdate(baseURL, updateDirectory, publicKeyPEM)
	if downloadError != nil {
		return
	}

	/* Use the new binary */
	switchToUpdatedVersion(false)

	return
}

func periodicUpdateRoutine(updateURL string) {
	if updateURL == "" {
		return
	}

	/*
	 * Public key(s) which may sign updates;  For now this is
	 * a single key.
	 */
	publicKeyPEM := []byte(`-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArpAZWeG0HHOGNALRj+UF
JLqXMJ2+AxFu7EWqQOVVh+LSpNn4kOm68TY4c4J4KsjRY9FlXXkZOv64oCBwIAXU
13+ciLIguJkJkSgOcuTuzBN1xPkWQdFRY1dkRv0Uc1AwEC5t89shkDbEB7z5uhyu
PHY/AnnudR60OP3Qe+HqxPLQAlhEvgHzGt0FktRRH+dcah+QycSWWjR3gEyDvKiO
keq8rrb+65AWob+AVIjiBN0SSqim9VxeEWIaMKOAjSBrUK9QahkPca+ZcrW5vIJX
E5M6PhOfWnxmqDGntZ/uJ+QEbTsUDisx4mEOen1JU6CgU8zqj1t3RGyDpWllCQT9
yQIDAQAB
-----END PUBLIC KEY-----`)

	currentVersion := sys.VersionCode

	baseURL := fmt.Sprintf("%s/network/%s/platform/%s", updateURL, sys.VersionMeta, sys.OSArch)

	for {
		checkForUpdate(baseURL, updateDirectory, publicKeyPEM, currentVersion)
		time.Sleep(10 * time.Minute)
	}
}

func switchToUpdatedVersion(atStartup bool) {
	/*
	 * Check for the new binary
	 */
	binaryFileName := path.Join(updateDirectory, "wavelet")
	_, err := os.Stat(binaryFileName)
	if err != nil {
		return
	}

	/*
	 * Re-exec with the new binary
	 */
	switchToUpdatedBinary(binaryFileName, atStartup)

	return
}
