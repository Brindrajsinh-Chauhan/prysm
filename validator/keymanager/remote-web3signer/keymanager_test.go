package remote_web3signer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer/internal"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer/v1/mock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type MockClient struct {
	Signature       string
	PublicKeys      []string
	isThrowingError bool
}

func (mc *MockClient) Sign(_ context.Context, _ string, _ internal.SignRequestJson) (bls.Signature, error) {
	decoded, err := hexutil.Decode(mc.Signature)
	if err != nil {
		return nil, err
	}
	return bls.SignatureFromBytes(decoded)
}
func (mc *MockClient) GetPublicKeys(_ context.Context, _ string) ([]string, error) {
	return mc.PublicKeys, nil
}

func TestNewKeymanager(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode([]string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"})
		require.NoError(t, err)
	}))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	tests := []struct {
		name    string
		args    *SetupConfig
		want    []string
		wantErr string
		wantLog string
	}{
		{
			name: "happy path public key url",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				PublicKeysURL:         srv.URL + "/public_keys",
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
		},
		{
			name: "bad public key url",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				PublicKeysURL:         "0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69",
			},
			wantErr: "could not get public keys from remote server url",
		},
		{
			name: "happy path provided public keys",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
		},
		{
			name: "path provided public keys, some bad key",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "http://prysm.xyz/"},
			},
			wantErr: "could not decode public key for remote signer",
		},
		{
			name: "path provided public keys, some bad hex for key",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937"},
			},
			wantErr: "invalid public key length",
		},
		{
			name: "happy path key file",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				KeyFilePath:           "./testing/good_keyfile.txt",
			},
			want: []string{"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
		},
		{
			name: "key file not found",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				KeyFilePath:           "./testing/invalid.txt",
			},
			wantLog: "Key file does not exist",
		},
		{
			name: "happy path public key url with good keyfile",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				PublicKeysURL:         srv.URL + "/public_keys",
				KeyFilePath:           "./testing/good_keyfile.txt",
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
		},
		{
			name: "happy path provided public keys with good keyfile",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logHook := logTest.NewGlobal()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			km, err := NewKeymanager(ctx, tt.args)
			if tt.wantLog != "" {
				require.LogsContain(t, logHook, tt.wantLog)
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			keys := make([]string, len(km.providedPublicKeys))
			for i, key := range km.providedPublicKeys {
				keys[i] = hexutil.Encode(key[:])
				require.Equal(t, true, slices.Contains(tt.want, keys[i]))
			}
		})
	}
}

func TestNewKeyManager_ChangingFileCreated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	bytesBuf := new(bytes.Buffer)
	_, err := bytesBuf.WriteString("8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055") // test without 0x
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	err = file.WriteFile(keyFilePath, bytesBuf.Bytes())
	require.NoError(t, err)

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
		ProvidedPublicKeys:    []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6"},
	})
	require.NoError(t, err)
	wantSlice := []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"}
	keys := make([]string, len(km.providedPublicKeys))
	require.Equal(t, 3, len(km.providedPublicKeys))
	for i, key := range km.providedPublicKeys {
		keys[i] = hexutil.Encode(key[:])
		require.Equal(t, slices.Contains(wantSlice, keys[i]), true)
	}
	// sleep needs to be at the front because of how watching the file works
	time.Sleep(1 * time.Second)

	bytesBuf = new(bytes.Buffer)
	_, err = bytesBuf.WriteString("0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	// Open the file for writing, create it if it does not exist, and truncate it if it does.
	f, err := os.OpenFile(keyFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	require.NoError(t, err)

	// Write the buffer's contents to the file.
	_, err = f.Write(bytesBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, f.Sync())
	require.NoError(t, f.Close())

	ks, _, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, 1, len(ks))
	require.Equal(t, "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", hexutil.Encode(ks[0][:]))

	require.Equal(t, 1, len(km.providedPublicKeys))
	require.Equal(t, "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", hexutil.Encode(km.providedPublicKeys[0][:]))
}

func TestKeymanager_Sign(t *testing.T) {
	client := &MockClient{
		Signature: "0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9",
	}
	ctx := context.Background()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         "http://example2.com/api/v1/eth2/publicKeys",
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	km.client = client
	desiredSigBytes, err := hexutil.Decode(client.Signature)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	desiredSig, err := bls.SignatureFromBytes(desiredSigBytes)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	type args struct {
		request *validatorpb.SignRequest
	}
	tests := []struct {
		name    string
		args    args
		want    bls.Signature
		wantErr bool
	}{
		{
			name: "AGGREGATION_SLOT",
			args: args{
				request: mock.GetMockSignRequest("AGGREGATION_SLOT"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "AGGREGATE_AND_PROOF",
			args: args{
				request: mock.GetMockSignRequest("AGGREGATE_AND_PROOF"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "ATTESTATION",
			args: args{
				request: mock.GetMockSignRequest("ATTESTATION"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "BLOCK",
			args: args{
				request: mock.GetMockSignRequest("BLOCK"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "BLOCK_V2",
			args: args{
				request: mock.GetMockSignRequest("BLOCK_V2"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "RANDAO_REVEAL",
			args: args{
				request: mock.GetMockSignRequest("RANDAO_REVEAL"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF",
			args: args{
				request: mock.GetMockSignRequest("SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "SYNC_COMMITTEE_MESSAGE",
			args: args{
				request: mock.GetMockSignRequest("SYNC_COMMITTEE_MESSAGE"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "SYNC_COMMITTEE_SELECTION_PROOF",
			args: args{
				request: mock.GetMockSignRequest("SYNC_COMMITTEE_SELECTION_PROOF"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "VOLUNTARY_EXIT",
			args: args{
				request: mock.GetMockSignRequest("VOLUNTARY_EXIT"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "VALIDATOR_REGISTRATION",
			args: args{
				request: mock.GetMockSignRequest("VALIDATOR_REGISTRATION"),
			},
			want:    desiredSig,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := km.Sign(ctx, tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("name:%s error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			require.DeepEqual(t, got, tt.want)
		})
	}

}

func TestKeymanager_FetchValidatingPublicKeys_HappyPath_WithKeyList(t *testing.T) {
	ctx := context.Background()
	decodedKey, err := hexutil.Decode("0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820")
	require.NoError(t, err)
	keys := [][48]byte{
		bytesutil.ToBytes48(decodedKey),
	}
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	resp, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	assert.NotNil(t, resp)
	assert.Nil(t, err)
	assert.EqualValues(t, resp, keys)
}

func TestKeymanager_FetchValidatingPublicKeys_HappyPath_WithExternalURL(t *testing.T) {
	ctx := context.Background()
	decodedKey, err := hexutil.Decode("0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	keys := [][48]byte{
		bytesutil.ToBytes48(decodedKey),
	}
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode([]string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"})
		require.NoError(t, err)
	}))
	defer srv.Close()
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	resp, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.EqualValues(t, resp, keys)
}

func TestKeymanager_FetchValidatingPublicKeys_WithExternalURL_ThrowsError(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "mock error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
	}
	km, err := NewKeymanager(ctx, config)
	require.ErrorContains(t, fmt.Sprintf("could not get public keys from remote server url: %s/api/v1/eth2/publicKeys", srv.URL), err)
	assert.Nil(t, km)
}

func TestKeymanager_AddPublicKeys(t *testing.T) {
	ctx := context.Background()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}
	statuses, err = km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusDuplicate, status.Status)
	}
}

func TestKeymanager_AddPublicKeys_WithFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}
	statuses, err = km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusDuplicate, status.Status)
	}
	keys, _, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, len(keys), len(publicKeys))
	require.Equal(t, hexutil.Encode(keys[0][:]), publicKeys[0])
}

func TestKeymanager_DeletePublicKeys(t *testing.T) {
	ctx := context.Background()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}

	s, err := km.DeletePublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusDeleted, status.Status)
	}

	s, err = km.DeletePublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusNotFound, status.Status)
	}
}

func TestKeymanager_DeletePublicKeys_WithFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
	}
	km, err := NewKeymanager(ctx, config)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}

	s, err := km.DeletePublicKeys([]string{publicKeys[0]})
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusDeleted, status.Status)
	}

	s, err = km.DeletePublicKeys([]string{publicKeys[0]})
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusNotFound, status.Status)
	}
	keys, _, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, len(keys), 1)
	require.Equal(t, hexutil.Encode(keys[0][:]), publicKeys[1])
}
