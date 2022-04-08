package keyshareserver

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/privacybydesign/gabi/signed"
	irma "github.com/privacybydesign/irmago"
	"github.com/privacybydesign/irmago/internal/keysharecore"
	"github.com/privacybydesign/irmago/internal/test"
	"github.com/privacybydesign/irmago/server"
	"github.com/privacybydesign/irmago/server/keyshare"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	irma.Logger.SetLevel(logrus.FatalLevel)
}

func TestServerInvalidMessage(t *testing.T) {
	keyshareServer, httpServer := StartKeyshareServer(t, NewMemoryDB(), "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	test.HTTPPost(t, nil, "http://localhost:8080/client/register",
		"gval;kefsajsdkl;", nil,
		400, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/users/start_auth",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		400, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		400, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin_challengeresponse",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		400, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/users/change/pin",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		400, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		400, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getCommitments",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		403, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getCommitments",
		"[]", nil,
		403, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getResponse",
		"asdlkzdsf;lskajl;kasdjfvl;jzxclvyewr", nil,
		403, nil,
	)
}

func TestServerHandleRegisterLegacy(t *testing.T) {
	keyshareServer, httpServer := StartKeyshareServer(t, NewMemoryDB(), "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	test.HTTPPost(t, nil, "http://localhost:8080/client/register",
		`{"pin":"testpin","email":"test@test.com","language":"en"}`, nil,
		200, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/client/register",
		`{"pin":"testpin","email":"test@test.com","language":"nonexistinglanguage"}`, nil,
		200, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/client/register",
		`{"pin":"testpin","language":"en"}`, nil,
		200, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/client/register",
		`{"pin":"testpin","language":"nonexistinglanguage"}`, nil,
		200, nil,
	)
	test.HTTPPost(t, nil, "http://localhost:8080/client/register",
		`{"pin":"testpin","language":"nonexistinglanguage"}`, nil,
		200, nil,
	)
}

func TestServerHandleRegister(t *testing.T) {
	keyshareServer, httpServer := StartKeyshareServer(t, NewMemoryDB(), "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	sk, err := signed.GenerateKey()
	require.NoError(t, err)
	pkbts, err := signed.MarshalPublicKey(&sk.PublicKey)
	require.NoError(t, err)

	j, err := jwt.NewWithClaims(jwt.SigningMethodES256, irma.KeyshareEnrollmentClaims{
		KeyshareEnrollmentData: irma.KeyshareEnrollmentData{
			Pin: "testpin", Language: "en", PublicKey: pkbts,
		},
	}).SignedString(sk)
	require.NoError(t, err)

	msg, err := json.Marshal(irma.KeyshareEnrollment{EnrollmentJWT: j})
	require.NoError(t, err)
	test.HTTPPost(t, nil, "http://localhost:8080/client/register", string(msg), nil, 200, nil)

	// Strip off a character to invalidate the JWT signature
	j = j[:len(j)-1]
	msg, err = json.Marshal(irma.KeyshareEnrollment{EnrollmentJWT: j})
	require.NoError(t, err)
	test.HTTPPost(t, nil, "http://localhost:8080/client/register", string(msg), nil, 500, nil)
}

func TestPinTries(t *testing.T) {
	db := createDB(t)
	keyshareServer, httpServer := StartKeyshareServer(t, &testDB{db: db, ok: true, tries: 1, wait: 0, err: nil}, "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	var jwtMsg irma.KeysharePinStatus
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
		`{"id":"legacyuser","pin":"puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n"}`, nil,
		200, &jwtMsg,
	)
	require.Equal(t, "success", jwtMsg.Status)

	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
		`{"id":"legacyuser","pin":"puZGbaLDmFywGhFDi4vW2G87Zh"}`, nil,
		200, &jwtMsg,
	)
	require.Equal(t, "failure", jwtMsg.Status)
	require.Equal(t, "1", jwtMsg.Message)

	test.HTTPPost(t, nil, "http://localhost:8080/users/change/pin",
		`{"id":"legacyuser","oldpin":"puZGbaLDmFywGhFDi4vW2G87Zh","newpin":"ljaksdfj;alkf"}`, nil,
		200, &jwtMsg,
	)
	require.Equal(t, "failure", jwtMsg.Status)
	require.Equal(t, "1", jwtMsg.Message)
}

func TestPinTryChallengeResponse(t *testing.T) {
	db := createDB(t)
	keyshareServer, httpServer := StartKeyshareServer(t, &testDB{db: db, ok: true, tries: 1, wait: 0, err: nil}, "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	// can't do this directly, challenge-response required
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
		`{"id":"testusername","pin":"puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n"}`, nil,
		500, nil,
	)

	sk := loadClientPrivateKey(t)

	res, err := json.Marshal(irma.KeyshareAuthResponse{
		Username: "testusername",
		Pin:      "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		Response: doChallengeResponse(t, sk),
	})
	require.NoError(t, err)
	var jwtMsg irma.KeysharePinStatus
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin_challengeresponse", string(res), nil, 200, &jwtMsg)
	require.Equal(t, "success", jwtMsg.Status)

	// try with an invalid response
	response := doChallengeResponse(t, sk)
	response[0] = ^response[0]
	res, err = json.Marshal(irma.KeyshareAuthResponse{
		Username: "testusername",
		Pin:      "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		Response: response,
	})
	require.NoError(t, err)
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin_challengeresponse", string(res), nil, 500, nil)
}

func TestStartAuth(t *testing.T) {
	db := createDB(t)
	keyshareServer, httpServer := StartKeyshareServer(t, &testDB{db: db, ok: true, tries: 1, wait: 0, err: nil}, "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	sk := loadClientPrivateKey(t)

	// do a full flow to get a JWT
	res, err := json.Marshal(irma.KeyshareAuthResponse{
		Username: "testusername",
		Pin:      "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		Response: doChallengeResponse(t, sk),
	})
	require.NoError(t, err)
	var jwtMsg irma.KeysharePinStatus
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin_challengeresponse", string(res), nil, 200, &jwtMsg)
	require.Equal(t, "success", jwtMsg.Status)
	require.Equal(t, "ey", jwtMsg.Message[:2]) // JWT's always start with ey

	// the JWT we just got authorizes us
	auth := &irma.KeyshareAuthChallenge{}
	test.HTTPPost(t, nil, "http://localhost:8080/users/start_auth",
		fmt.Sprintf(`{"id":"testusername","jwt":"%s"}`, jwtMsg.Message), nil,
		200, auth,
	)
	require.Equal(t, auth.Status, "authorized")
	require.Empty(t, auth.Candidates)

	// nonexisting user
	test.HTTPPost(t, nil, "http://localhost:8080/users/start_auth",
		`{"id":"doesnotexist"}`, nil,
		403, nil,
	)
}

func TestRegisterPublicKey(t *testing.T) {
	db := createDB(t)
	keyshareServer, httpServer := StartKeyshareServer(t, &testDB{db: db, ok: true, tries: 1, wait: 0, err: nil}, "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	sk := loadClientPrivateKey(t)
	pk, err := signed.MarshalPublicKey(&sk.PublicKey)
	require.NoError(t, err)

	// first try with nonexisting user
	jwtt := registrationJWT(t, sk, irma.KeysharePublicKeyRegistrationData{
		Username:  "doesnotexist",
		Pin:       "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		PublicKey: pk,
	})
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		fmt.Sprintf(`{"jwt":"%s"}`, jwtt), nil,
		403, nil,
	)

	// then try with invalid jwt
	jwtt = registrationJWT(t, sk, irma.KeysharePublicKeyRegistrationData{
		Username:  "legacyuser",
		Pin:       "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		PublicKey: pk,
	})
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		fmt.Sprintf(`{"jwt":"%s"}`, jwtt[:len(jwtt)-1]), nil,
		400, nil,
	)

	// then try with wrong pin
	jwtt = registrationJWT(t, sk, irma.KeysharePublicKeyRegistrationData{
		Username:  "legacyuser",
		Pin:       "puZGbaLDmFywGhFDi4vW2G87Zh",
		PublicKey: pk,
	})
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		fmt.Sprintf(`{"jwt":"%s"}`, jwtt), nil,
		500, nil,
	)

	// normal flow
	jwtt = registrationJWT(t, sk, irma.KeysharePublicKeyRegistrationData{
		Username:  "legacyuser",
		Pin:       "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		PublicKey: pk,
	})
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		fmt.Sprintf(`{"jwt":"%s"}`, jwtt), nil,
		200, nil,
	)

	// challenge-response should work now
	_ = doChallengeResponse(t, loadClientPrivateKey(t))

	// can't do it a second time
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		fmt.Sprintf(`{"jwt":"%s"}`, jwtt), nil,
		500, nil,
	)
}

func TestRegisterPublicKeyBlockedUser(t *testing.T) {
	db := createDB(t)
	keyshareServer, httpServer := StartKeyshareServer(t, &testDB{db: db, ok: false, tries: 0, wait: 5, err: nil}, "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	sk := loadClientPrivateKey(t)
	pk, err := signed.MarshalPublicKey(&sk.PublicKey)
	require.NoError(t, err)

	// submit wrong pin, blocking user
	var jwtMsg irma.KeysharePinStatus
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
		`{"id":"legacyuser","pin":"puZGbaLDmFywGhFDi4vW2G87Zh"}`, nil,
		200, &jwtMsg,
	)
	require.Equal(t, "error", jwtMsg.Status)
	require.Equal(t, "5", jwtMsg.Message)

	// try to register public key
	jwtt := registrationJWT(t, sk, irma.KeysharePublicKeyRegistrationData{
		Username:  "legacyuser",
		Pin:       "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		PublicKey: pk,
	})
	test.HTTPPost(t, nil, "http://localhost:8080/users/register_publickey",
		fmt.Sprintf(`{"jwt":"%s"}`, jwtt), nil,
		200, &jwtMsg,
	)
	require.Equal(t, "error", jwtMsg.Status)
}

func TestPinNoRemainingTries(t *testing.T) {
	db := createDB(t)

	for _, ok := range []bool{true, false} {
		keyshareServer, httpServer := StartKeyshareServer(t, &testDB{db: db, ok: ok, tries: 0, wait: 5, err: nil}, "")

		var jwtMsg irma.KeysharePinStatus
		test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
			`{"id":"testusername","pin":"puZGbaLDmFywGhFDi4vW2G87Zh"}`, nil,
			200, &jwtMsg,
		)
		require.Equal(t, "error", jwtMsg.Status)
		require.Equal(t, "5", jwtMsg.Message)

		test.HTTPPost(t, nil, "http://localhost:8080/users/change/pin",
			`{"id":"testusername","oldpin":"puZGbaLDmFywGhFDi4vW2G87Zh","newpin":"ljaksdfj;alkf"}`, nil,
			200, &jwtMsg,
		)
		require.Equal(t, "error", jwtMsg.Status)
		require.Equal(t, "5", jwtMsg.Message)

		StopKeyshareServer(t, keyshareServer, httpServer)
	}
}

func TestMissingUser(t *testing.T) {
	keyshareServer, httpServer := StartKeyshareServer(t, NewMemoryDB(), "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin",
		`{"id":"doesnotexist","pin":"bla"}`, nil,
		403, nil,
	)

	test.HTTPPost(t, nil, "http://localhost:8080/users/change/pin",
		`{"id":"doesnotexist","oldpin":"old","newpin":"new"}`, nil,
		403, nil,
	)

	test.HTTPPost(t, nil, "http://localhost:8080/prove/getCommitments",
		`["test.test-3"]`, http.Header{
			"X-IRMA-Keyshare-Username": []string{"doesnotexist"},
			"Authorization":            []string{"ey.ey.ey"},
		},
		403, nil,
	)

	test.HTTPPost(t, nil, "http://localhost:8080/prove/getResponse",
		"123456789", http.Header{
			"X-IRMA-Keyshare-Username": []string{"doesnotexist"},
			"Authorization":            []string{"ey.ey.ey"},
		},
		403, nil,
	)
}

func TestKeyshareSessions(t *testing.T) {
	db := createDB(t)
	keyshareServer, httpServer := StartKeyshareServer(t, db, "")
	defer StopKeyshareServer(t, keyshareServer, httpServer)

	res, err := json.Marshal(irma.KeyshareAuthResponse{
		Username: "testusername",
		Pin:      "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
		Response: doChallengeResponse(t, loadClientPrivateKey(t)),
	})
	require.NoError(t, err)
	var jwtMsg irma.KeysharePinStatus
	test.HTTPPost(t, nil, "http://localhost:8080/users/verify/pin_challengeresponse", string(res), nil, 200, &jwtMsg)
	require.Equal(t, "success", jwtMsg.Status)

	// no active session, can't retrieve result
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getResponse",
		"12345678", http.Header{
			"X-IRMA-Keyshare-Username": []string{"testusername"},
			"Authorization":            []string{jwtMsg.Message},
		},
		400, nil,
	)

	// can't retrieve commitments with fake authorization
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getCommitments",
		`["test.test-3"]`, http.Header{
			"X-IRMA-Keyshare-Username": []string{"testusername"},
			"Authorization":            []string{"fakeauthorization"},
		},
		400, nil,
	)

	// retrieve commitments normally
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getCommitments",
		`["test.test-3"]`, http.Header{
			"X-IRMA-Keyshare-Username": []string{"testusername"},
			"Authorization":            []string{jwtMsg.Message},
		},
		200, nil,
	)

	// can't retrieve resukt with fake authorization
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getResponse",
		"12345678", http.Header{
			"X-IRMA-Keyshare-Username": []string{"testusername"},
			"Authorization":            []string{"fakeauthorization"},
		},
		400, nil,
	)

	// can start session while another is already active
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getCommitments",
		`["test.test-3"]`, http.Header{
			"X-IRMA-Keyshare-Username": []string{"testusername"},
			"Authorization":            []string{jwtMsg.Message},
		},
		200, nil,
	)

	// finish session
	test.HTTPPost(t, nil, "http://localhost:8080/prove/getResponse",
		"12345678", http.Header{
			"X-IRMA-Keyshare-Username": []string{"testusername"},
			"Authorization":            []string{jwtMsg.Message},
		},
		200, nil,
	)
}

func StartKeyshareServer(t *testing.T, db DB, emailserver string) (*Server, *http.Server) {
	testdataPath := test.FindTestdataFolder(t)
	s, err := New(&Configuration{
		Configuration: &server.Configuration{
			SchemesPath:           filepath.Join(testdataPath, "irma_configuration"),
			IssuerPrivateKeysPath: filepath.Join(testdataPath, "privatekeys"),
			Logger:                irma.Logger,
		},
		EmailConfiguration: keyshare.EmailConfiguration{
			EmailServer:     emailserver,
			EmailFrom:       "test@example.com",
			DefaultLanguage: "en",
		},
		DB:                    db,
		JwtKeyID:              0,
		JwtPrivateKeyFile:     filepath.Join(testdataPath, "jwtkeys", "kss-sk.pem"),
		StoragePrimaryKeyFile: filepath.Join(testdataPath, "keyshareStorageTestkey"),
		KeyshareAttribute:     irma.NewAttributeTypeIdentifier("test.test.mijnirma.email"),
		RegistrationEmailFiles: map[string]string{
			"en": filepath.Join(testdataPath, "emailtemplate.html"),
		},
		RegistrationEmailSubjects: map[string]string{
			"en": "testsubject",
		},
		VerificationURL: map[string]string{
			"en": "http://example.com/verify/",
		},
	})
	require.NoError(t, err)

	serv := &http.Server{
		Addr:    "localhost:8080",
		Handler: s.Handler(),
	}

	go func() {
		err := serv.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		assert.NoError(t, err)
	}()
	time.Sleep(200 * time.Millisecond) // Give server time to start

	return s, serv
}

func StopKeyshareServer(t *testing.T, keyshareServer *Server, httpServer *http.Server) {
	keyshareServer.Stop()
	err := httpServer.Shutdown(context.Background())
	assert.NoError(t, err)
}

type testDB struct {
	db    DB
	ok    bool
	tries int
	wait  int64
	err   error
}

func (db *testDB) AddUser(user *User) error {
	return db.db.AddUser(user)
}

func (db *testDB) user(username string) (*User, error) {
	return db.db.user(username)
}

func (db *testDB) updateUser(user *User) error {
	return db.db.updateUser(user)
}

func (db *testDB) reservePinTry(_ *User) (bool, int, int64, error) {
	return db.ok, db.tries, db.wait, db.err
}

func (db *testDB) resetPinTries(user *User) error {
	return db.db.resetPinTries(user)
}

func (db *testDB) setSeen(user *User) error {
	return db.db.setSeen(user)
}

func (db *testDB) addLog(user *User, entrytype eventType, params interface{}) error {
	return db.db.addLog(user, entrytype, params)
}

func (db *testDB) addEmailVerification(user *User, email, token string) error {
	return db.db.addEmailVerification(user, email, token)
}

func createDB(t *testing.T) DB {
	db := NewMemoryDB()
	err := db.AddUser(&User{
		Username: "",
		Secrets:  keysharecore.UserSecrets{},
	})
	require.NoError(t, err)
	secrets, err := base64.StdEncoding.DecodeString("YWJjZBdd6z/4lW/JBgEjVxcAnhK16iimfeyi1AAtWPzkfbWYyXHAad8A+Xzc6mE8bMj6dMQ5CgT0xcppEWYN9RFtO5+Wv4Carfq3TEIX9IWEDuU+lQG0noeHzKZ6k1J22iNAiL7fEXNWNy2H7igzJbj6svbH2LTRKxEW2Cj9Qkqzip5UapHmGZf6G6E7VkMvmJsbrW5uoZAVq2vP+ocuKmzBPaBlqko9F0YKglwXyhfaQQQ0Y3x4secMwC12")
	require.NoError(t, err)
	err = db.AddUser(&User{
		Username: "legacyuser",
		Secrets:  secrets,
	})
	require.NoError(t, err)

	secrets, err = base64.StdEncoding.DecodeString("YWJjZHpSayGYcjcKbUNfJJjNOXxgxV+GWTVYinpeKqTSfUjUuT4+Hs2uZY68+KvnXkPkoV1eo4HvpVzxy683DHi8Ih+P4Nuqz4FhhLddFnZlzPn1sHuvSjs8S2qGP/jO5+3075I/TWiT2CxO8B83ezMX7tmlwvTbWdYbmV1saEyCVFssuzTARcfvee0f6YvFe9eX1iHfAwXvPsdrt0eTqbTcUzDzv5pQb/t18MtJsK6cB2vh3XJO0psbBWsshGNJYIkMaiGmhi457zejvIt1xcC+dsZZUJVpvoGrZvHd25gH9PLQ/VSU0atrhXS93nsdW8+Y4M4tDFZ8R9pZsseZKt4Zuj1FbxD/qZcdm2w8KaCQgVjzzJJu6//Z5/qF0Neycmm6uiAs4zQWVkibtR9BLEmwHsLd2u4n1EhPAzp14kyzI72/")
	require.NoError(t, err)
	err = db.AddUser(&User{
		Username: "testusername",
		Secrets:  secrets,
	})
	require.NoError(t, err)

	return db
}

func doChallengeResponse(t *testing.T, sk *ecdsa.PrivateKey) []byte {
	// retrieve a challenge
	auth := &irma.KeyshareAuthChallenge{}
	test.HTTPPost(t, nil, "http://localhost:8080/users/start_auth",
		`{"id":"testusername"}`, nil,
		200, auth,
	)
	require.Equal(t, auth.Status, "invalid")
	require.Contains(t, auth.Candidates, irma.KeyshareAuthMethodECDSA)
	require.NotEmpty(t, auth.Challenge)

	// sign the challenge
	msg, err := json.Marshal(irma.KeyshareChallengeData{
		Challenge: auth.Challenge,
		PIN:       "puZGbaLDmFywGhFDi4vW2G87ZhXpaUsvymZwNJfB/SU=\n",
	})
	require.NoError(t, err)
	response, err := signed.Sign(sk, msg)
	require.NoError(t, err)

	return response
}

func loadClientPrivateKey(t *testing.T) *ecdsa.PrivateKey {
	testdata := test.FindTestdataFolder(t)
	bts, err := os.ReadFile(filepath.Join(testdata, "client", "ecdsa_sk.pem"))
	require.NoError(t, err)
	sk, err := signed.UnmarshalPemPrivateKey(bts)
	require.NoError(t, err)
	return sk
}

func registrationJWT(t *testing.T, sk *ecdsa.PrivateKey, data irma.KeysharePublicKeyRegistrationData) string {
	j, err := jwt.NewWithClaims(jwt.SigningMethodES256, irma.KeysharePublicKeyRegistrationClaims{
		KeysharePublicKeyRegistrationData: data,
	}).SignedString(sk)
	require.NoError(t, err)
	return j
}
