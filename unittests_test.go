package unittests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/geraldhinson/siftd-base/pkg/security"
	"github.com/geraldhinson/siftd-base/pkg/serviceBase"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/constants"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/models"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/queryhelpers"
	"github.com/spf13/viper"
)

// this is a map of query defined auth property to actual auth policies.
// NOTE: The map key string *must* match one of the auth properties defined in the queries files
var policyTranslation = &models.QueryFileAuthPoliciesList{
	"machine realm: valid identity": {
		Realm:        security.REALM_MACHINE,
		AuthType:     security.VALID_IDENTITY,
		Timeout:      security.ONE_HOUR,
		ApprovedList: nil,
	},
	"member realm: valid identity matching the url ownerid": {
		Realm:        security.REALM_MEMBER,
		AuthType:     security.MATCHING_IDENTITY,
		Timeout:      security.ONE_DAY,
		ApprovedList: nil,
	},
	"member realm: approved groups": {
		Realm:        security.REALM_MEMBER,
		AuthType:     security.APPROVED_GROUPS,
		Timeout:      security.ONE_DAY,
		ApprovedList: []string{"admin"},
	},
	"public access": {
		Realm:        security.NO_REALM,
		AuthType:     security.NO_AUTH,
		Timeout:      security.NO_EXPIRY,
		ApprovedList: nil,
	},
}

func TestCalls(t *testing.T) {
	router, err := NewQueriesService()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}

	t.Run("GET health - valid method", func(t *testing.T) {
		body, err, status := CallServiceViaLoopback(router.Configuration, "v1/health")
		if err != nil {
			t.Fatalf("Failed to call public queries router via loopback: %v, %d", err, status)
		}
		if status != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, status)
		}
		if body == nil {
			t.Fatalf("Expected body, got nil")
		}
		if strings.Contains(string(body), "UNHEALTHY") {
			fmt.Println("Health check response received was: ", string(body))
			t.Fatalf("Expected HEALTHY status, got UNHEALTHY")
		}
	})

	t.Run("GET public queries request - valid request", func(t *testing.T) {
		body, err, status := CallServiceViaLoopback(router.Configuration, "v1/public/queries")
		if err != nil {
			t.Fatalf("Failed to call public queries router via loopback: %v, %d", err, status)
		}
		if status != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, status)
		}
		if body == nil {
			t.Fatalf("Expected content returned, got nil")
		}
	})

	t.Run("GET counties per state request - valid method / undefined SPROC", func(t *testing.T) {
		body, err, status := CallServiceViaLoopback(router.Configuration, "v1/public/queries/unittests/getStateCountyMap?states=[\"CA\",\"TX\"]")
		if err != nil {
			t.Fatalf("Failed to call public queries router via loopback: %v, %d", err, status)
		}
		if status != http.StatusInternalServerError {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, status)
		}
		if body == nil {
			t.Fatalf("Expected body, got nil")
		}
		if !strings.Contains(string(body), "Internal Server Error:") {
			t.Fatalf("Expected body to contain 'Internal Server Error:', got %s", string(body))
		}
	})

	t.Run("GET json by id - valid method", func(t *testing.T) {
		body, err, status := CallServiceViaLoopback(router.Configuration, "v1/queries/unittests/getJsonById?id=1")
		if err != nil {
			t.Fatalf("Failed to call secured queries router via loopback: %v, %d", err, status)
		}
		if status != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, status)
		}
		if body == nil {
			t.Fatalf("Expected body, got nil")
		}
		if !strings.Contains(string(body), "aJson") {
			t.Fatalf("Expected body to contain 'aJson' root node in json returned, got %s", string(body))
		}
	})

	t.Run("GET private/secured queries request - valid request", func(t *testing.T) {
		body, err, status := CallServiceViaLoopback(router.Configuration, "v1/queries")
		if err != nil {
			t.Fatalf("Failed to call public queries router via loopback: %v, %d", err, status)
		}
		if status != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, status)
		}
		if body == nil {
			t.Fatalf("Expected content returned, got nil")
		}
	})

	t.Run("GET data by ownerId - valid method", func(t *testing.T) {
		body, err, status := CallServiceViaLoopback(router.Configuration, "v1/identities/GUID-fake-member-GUID/queries/unittests/getDataByOwnerId")
		if err != nil {
			t.Fatalf("Failed to call secured queries router via loopback: %v, %d", err, status)
		}
		if status != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, status)
		}
		if body == nil {
			t.Fatalf("Expected body, got nil")
		}
		if !strings.Contains(string(body), "GUID-fake-member-GUID") {
			t.Fatalf("Expected body to contain 'aJson' root node in json returned, got %s", string(body))
		}
	})
}

func TestShutdownListener(t *testing.T) {
	t.Logf("Shutting down listener...")

	// Assuming the current process is the one to be shut down
	cmd := syscall.Getpid()
	process, err := os.FindProcess(cmd)
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}
	if err := process.Signal(syscall.SIGINT); err != nil {
		panic(err)
	}

	t.Logf("waiting 1 seconds to see shutdown")
	time.Sleep(1 * time.Second)
}

func NewQueriesService() (*queryhelpers.PublicQueriesRouter, error) {

	// call setup to get the service base (logging, config, routing) and a keystore
	queryService := serviceBase.NewServiceBase()
	if queryService == nil {
		return nil, fmt.Errorf("Failed to validate configuration and listen. Shutting down.")
	}

	PublicQueriesRouter := queryhelpers.NewPublicQueriesRouter(queryService, policyTranslation)
	//security.NO_REALM, security.NO_AUTH, security.NO_EXPIRY, nil)
	if PublicQueriesRouter == nil {
		return nil, fmt.Errorf("Failed to create public queries api server. Shutting down.")
	}

	SecuredQueriesRouter := queryhelpers.NewSecuredQueriesRouter(queryService, policyTranslation)
	if SecuredQueriesRouter == nil {
		queryService.Logger.Fatalf("Failed to create secured queries api server. Shutting down.")
		return nil, fmt.Errorf("Failed to create secured queries api server. Shutting down.")
	}

	HealthCheckRouter := queryhelpers.NewHealthCheckRouter(queryService, security.NO_REALM, security.NO_AUTH, security.NO_EXPIRY, nil)
	if HealthCheckRouter == nil {
		return nil, fmt.Errorf("Failed to create health check api server. Shutting down.")
	}

	/*
		// setup
		listenAddress := queryService.Configuration.GetString(constants.LISTEN_ADDRESS)
		if listenAddress == "" {
			queryService.Logger.Fatalf("Unable to retrieve listen address and port. Shutting down.")
			return
		}

		// if we are running on localhost, we can add a fake identity service for testing (id is hardcoded in FakeKeyStore.go)
		if strings.Contains(listenAddress, "localhost") {
			FakeIdentityServiceRouter := helpers.NewFakeIdentityServiceRouter(queryService, security.NO_REALM, security.NO_AUTH, security.NO_EXPIRY, nil)
			if FakeIdentityServiceRouter == nil {
				queryService.Logger.Fatalf("Failed to create fake identity service api server (for testing only). Shutting down.")
				return
			}
		}
	*/

	go queryService.ListenAndServe()

	return PublicQueriesRouter, nil
}

func CallServiceViaLoopback(configuration *viper.Viper, requestURLSuffix string) ([]byte, error, int) {

	listenAddress := configuration.GetString(constants.LISTEN_ADDRESS)
	if listenAddress == "" {
		err := fmt.Errorf("Unable to retrieve listen address and port. Shutting down.")
		return nil, err, http.StatusBadRequest
	}
	requestURL := fmt.Sprintf("%s/%s", listenAddress, requestURLSuffix)

	req, err := http.NewRequest(constants.HTTP_GET, requestURL, nil)
	if err != nil {
		err = fmt.Errorf("failed to build noun service request in UnitTest: %s", err)
		return nil, err, http.StatusBadRequest
	}
	//	if (fakeUserToken != nil) && (len(fakeUserToken) > 0) {
	//		req.Header.Add("Authorization", string(fakeUserToken))
	//	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("client call to noun service failed with : %s", err)
		return nil, err, http.StatusBadRequest
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("unable to read noun service reply: %s", err)
		return nil, err, http.StatusBadRequest
	}

	return resBody, err, res.StatusCode
}
