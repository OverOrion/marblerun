// Copyright (c) Edgeless Systems GmbH.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package core

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/edgelesssys/marblerun/coordinator/quote"
	"github.com/edgelesssys/marblerun/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustSetup() (*Core, *Manifest) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(test.ManifestJSON), &manifest); err != nil {
		panic(err)
	}
	return NewCoreWithMocks(), &manifest
}

func setupTestCerts(key *rsa.PrivateKey) (*x509.Certificate, *x509.Certificate) {
	// Create some demo certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1337),
		IsCA:         false,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365),
	}

	otherTestCertRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	otherTestCert, err := x509.ParseCertificate(otherTestCertRaw)
	if err != nil {
		panic(err)
	}

	block, _ := pem.Decode([]byte(test.AdminCert))
	adminTestCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic(err)
	}

	return adminTestCert, otherTestCert
}

func TestGetManifestSignature(t *testing.T) {
	assert := assert.New(t)

	c, _ := mustSetup()

	_, err := c.SetManifest(context.TODO(), []byte(test.ManifestJSON))

	assert.NoError(err)

	sig := c.GetManifestSignature(context.TODO())
	expectedHash := sha256.Sum256([]byte(test.ManifestJSON))
	assert.Equal(expectedHash[:], sig)
}

func TestSetManifest(t *testing.T) {
	assert := assert.New(t)

	c, manifest := mustSetup()
	_, err := c.SetManifest(context.TODO(), []byte(test.ManifestJSON))

	assert.NoError(err, "SetManifest should succed on first try")
	assert.Equal(*manifest, c.manifest, "Manifest should be set correctly")
	_, err = c.SetManifest(context.TODO(), []byte(test.ManifestJSON))
	assert.Error(err, "SetManifest should fail on the second try")
	assert.Equal(*manifest, c.manifest, "Manifest should still be set correctly")
	_, err = c.SetManifest(context.TODO(), []byte(test.ManifestJSON)[:len(test.ManifestJSON)-1])
	assert.Error(err, "SetManifest should fail on broken json")
	assert.Equal(*manifest, c.manifest, "Manifest should still be set correctly")

	// use new core
	c, _ = mustSetup()
	_, err = c.SetManifest(context.TODO(), []byte(test.ManifestJSON)[:len(test.ManifestJSON)-1])
	assert.Error(err, "SetManifest should fail on broken json")
	c, _ = mustSetup()
	_, err = c.SetManifest(context.TODO(), []byte(""))
	assert.Error(err, "empty string should not be accepted")
	_, err = c.SetManifest(context.TODO(), []byte(test.ManifestJSON))
	assert.NoError(err, "SetManifest should succed after failed tries")
	assert.Equal(*manifest, c.manifest, "Manifest should be set correctly")
}

func TestSetManifestInvalid(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// try setting manifest with unallowed marble package, but proper json
	c, manifest := mustSetup()
	// get any element of the map
	for _, marble := range manifest.Marbles {
		marble.Package = "foo"
		manifest.Marbles["bar"] = marble
		break
	}
	modRawManifest, err := json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.Equal("manifest does not contain marble package foo", err.Error())

	// Try setting manifest with all values unset, no debug mode (this should fail)
	c, manifest = mustSetup()

	backendPackage := manifest.Packages["backend"]
	backendPackage.Debug = false
	backendPackage.UniqueID = ""
	backendPackage.SignerID = ""
	backendPackage.ProductID = nil
	backendPackage.SecurityVersion = nil

	manifest.Packages["backend"] = backendPackage
	modRawManifest, err = json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.Equal("manifest misses value for SignerID in package backend", err.Error())

	// Enable debug mode, should work now
	c = testManifestInvalidDebugCase(c, manifest, backendPackage, assert, require)

	// Set SignerID, now should complain about missing ProductID
	backendPackage.SignerID = "some signer"
	manifest.Packages["backend"] = backendPackage

	modRawManifest, err = json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.Equal("manifest misses value for ProductID in package backend", err.Error())

	// Enable debug mode, should work now
	c = testManifestInvalidDebugCase(c, manifest, backendPackage, assert, require)

	// Set ProductID, now should complain about missing SecurityVersion
	productIDValue := uint64(42)
	backendPackage.ProductID = &productIDValue
	manifest.Packages["backend"] = backendPackage

	modRawManifest, err = json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.Equal("manifest misses value for SecurityVersion in package backend", err.Error())

	// Enable debug mode, should work now
	c = testManifestInvalidDebugCase(c, manifest, backendPackage, assert, require)

	// Set SecurityVersion, now we should pass
	securityVersion := uint(1)
	backendPackage.SecurityVersion = &securityVersion
	manifest.Packages["backend"] = backendPackage

	modRawManifest, err = json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.NoError(err)

	// Reset & enable debug mode, should also work now
	c, _ = mustSetup()
	c = testManifestInvalidDebugCase(c, manifest, backendPackage, assert, require)

	// Try setting manifest with UniqueID + other value set, this should fail again
	backendPackage.UniqueID = "something unique"
	manifest.Packages["backend"] = backendPackage

	modRawManifest, err = json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.Equal("manifest specfies both UniqueID *and* SignerID/ProductID/SecurityVersion in package backend", err.Error())

	// Enable debug mode, should work now
	c = testManifestInvalidDebugCase(c, manifest, backendPackage, assert, require)
}

func TestGetCertQuote(t *testing.T) {
	assert := assert.New(t)

	c, _ := mustSetup()

	cert, _, err := c.GetCertQuote(context.TODO())
	assert.NoError(err, "GetCertQuote should not fail (without manifest)")
	assert.Contains(cert, "-----BEGIN CERTIFICATE-----", "simple format check")

	c.SetManifest(context.TODO(), []byte(test.ManifestJSON))
	_, _, err = c.GetCertQuote(context.TODO())
	assert.NoError(err, "GetCertQuote should not fail (with manifest)")
	//todo check quote
}

func TestGetStatus(t *testing.T) {
	assert := assert.New(t)
	c, _ := mustSetup()

	// Server should be ready to accept a manifest after initializing a mock core
	statusCode, status, err := c.GetStatus(context.TODO())
	assert.NoError(err, "GetStatus failed")
	assert.EqualValues(stateAcceptingManifest, statusCode, "We should be ready to accept a manifest now, but GetStatus does tell us we don't.")
	assert.NotEmpty(status, "Status string was empty, but should not.")

	// Set a manifest, state should change
	_, err = c.SetManifest(context.TODO(), []byte(test.ManifestJSON))
	statusCode, status, err = c.GetStatus(context.TODO())
	assert.NoError(err, "GetStatus failed")
	assert.EqualValues(stateAcceptingMarbles, statusCode, "We should be ready to accept Marbles now, but GetStatus does tell us we don't.")
	assert.NotEmpty(status, "Status string was empty, but should not.")
}

func TestVerifyAdmin(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	c, _ := mustSetup()

	adminTestCert, otherTestCert := setupTestCerts(test.RecoveryPrivateKey)

	// Set a manifest containing an admin certificate
	_, err := c.SetManifest(context.TODO(), []byte(test.ManifestJSONWithRecoveryKey))
	require.NoError(err)
	adminTestCertSlice := make([]*x509.Certificate, 1)
	otherTestCertSlice := make([]*x509.Certificate, 1)

	// Put certificates in slice, as Go's TLS library passes them in an HTTP request
	adminTestCertSlice[0] = adminTestCert
	otherTestCertSlice[0] = otherTestCert

	// Check if the adminTest certificatge is deemed valid (stored in core), and the freshly generated one is deemed false
	assert.Equal(true, c.VerifyAdmin(context.TODO(), adminTestCertSlice))
	assert.Equal(false, c.VerifyAdmin(context.TODO(), otherTestCertSlice))
	assert.Equal(false, c.VerifyAdmin(context.TODO(), nil))
}

func TestUpdateManifest(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	c, _ := mustSetup()

	// Set manifest (frontend has SecurityVersion 3)
	_, err := c.SetManifest(context.TODO(), []byte(test.ManifestJSON))
	require.NoError(err)
	oldValue := *c.manifest.Packages["frontend"].SecurityVersion

	// Try to update manifest (frontend's SecurityVersion should rise from 3 to 5)
	err = c.UpdateManifest(context.TODO(), []byte(test.UpdateManifest))
	require.NoError(err)
	newValue := *c.updateManifest.Packages["frontend"].SecurityVersion

	// Check if the value did indeed rise
	assert.Greater(newValue, oldValue)
}

func testManifestInvalidDebugCase(c *Core, manifest *Manifest, marblePackage quote.PackageProperties, assert *assert.Assertions, require *require.Assertions) *Core {
	marblePackage.Debug = true
	manifest.Packages["backend"] = marblePackage

	modRawManifest, err := json.Marshal(manifest)
	require.NoError(err)
	_, err = c.SetManifest(context.TODO(), modRawManifest)
	assert.NoError(err)
	marblePackage.Debug = false

	// Since debug case should pass, return a resetted fresh core
	c, _ = mustSetup()
	return c
}
