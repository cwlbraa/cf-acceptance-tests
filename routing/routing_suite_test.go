package routing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/app_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"

	"testing"
)

const (
	DEFAULT_MEMORY_LIMIT = "256M"
)

var (
	DEFAULT_TIMEOUT = 30 * time.Second
	CF_PUSH_TIMEOUT = 2 * time.Minute

	config helpers.Config
)

type AppResource struct {
	Metadata struct {
		Url string
	}
}
type AppsResponse struct {
	Resources []AppResource
}
type Stat struct {
	Stats struct {
		Host string
		Port int
	}
}
type StatsResponse map[string]Stat

func EnableDiego(appName string) {
	appGuid := GetAppGuid(appName)
	Expect(cf.Cf("curl", fmt.Sprintf("/v2/apps/%s", appGuid), "-d", `{"diego": true}`, "-X", "PUT").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
}

func RestartApp(app string) {
	Expect(cf.Cf("restart", app).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
}

func StartApp(app string) {
	app_helpers.SetBackend(app)
	Expect(cf.Cf("start", app).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
}

func PushApp(asset, buildpackName string) string {
	app := PushAppNoStart(asset, buildpackName)
	StartApp(app)
	return app
}

func PushAppNoStart(asset, buildpackName string) string {
	app := generator.PrefixedRandomName("RATS-APP-")
	Expect(cf.Cf("push", app, "-b", buildpackName, "--no-start", "-m", DEFAULT_MEMORY_LIMIT, "-p", asset, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
	return app
}

func ScaleAppInstances(appName string, instances int) {
	Expect(cf.Cf("scale", appName, "-i", strconv.Itoa(instances)).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
	Eventually(func() string {
		return string(cf.Cf("app", appName).Wait(DEFAULT_TIMEOUT).Out.Contents())
	}, DEFAULT_TIMEOUT*2, 2*time.Second).
		Should(ContainSubstring(fmt.Sprintf("instances: %d/%d", instances, instances)))
}

func DeleteApp(appName string) {
	Expect(cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
}

func GetAppGuid(appName string) string {
	session := cf.Cf("app", appName, "--guid").Wait(DEFAULT_TIMEOUT)
	Expect(session).To(Exit(0))
	appGuid := session.Out.Contents()
	return strings.TrimSpace(string(appGuid))
}

func MapRouteToApp(domain, path, app string) {
	spaceGuid, domainGuid := GetSpaceAndDomainGuids(app)

	routeGuid := CreateRoute(domain, path, spaceGuid, domainGuid)
	appGuid := GetAppGuid(app)

	Expect(cf.Cf("curl", "/v2/apps/"+appGuid+"/routes/"+routeGuid, "-X", "PUT").Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
}

func CreateRoute(domainName, contextPath, spaceGuid, domainGuid string) string {
	jsonBody := "{\"host\":\"" + domainName + "\", \"path\":\"" + contextPath + "\", \"domain_guid\":\"" + domainGuid + "\",\"space_guid\":\"" + spaceGuid + "\"}"
	session := cf.Cf("curl", "/v2/routes", "-X", "POST", "-d", jsonBody).Wait(CF_PUSH_TIMEOUT)
	routePostResponseBody := session.Out.Contents()

	var routeResponseJSON struct {
		Metadata struct {
			Guid string `json:"guid"`
		} `json:"metadata"`
	}
	err := json.Unmarshal([]byte(routePostResponseBody), &routeResponseJSON)
	Expect(err).NotTo(HaveOccurred())
	return routeResponseJSON.Metadata.Guid
}

func GetSpaceAndDomainGuids(app string) (string, string) {
	getRoutePath := fmt.Sprintf("/v2/routes?q=host:%s", app)
	routeBody := cf.Cf("curl", getRoutePath).Wait(DEFAULT_TIMEOUT).Out.Contents()
	var routeJSON struct {
		Resources []struct {
			Entity struct {
				SpaceGuid  string `json:"space_guid"`
				DomainGuid string `json:"domain_guid"`
			} `json:"entity"`
		} `json:"resources"`
	}

	err := json.Unmarshal([]byte(routeBody), &routeJSON)
	Expect(err).NotTo(HaveOccurred())

	spaceGuid := routeJSON.Resources[0].Entity.SpaceGuid
	domainGuid := routeJSON.Resources[0].Entity.DomainGuid

	return spaceGuid, domainGuid
}

func GetAppInfo(appName string) (host, port string) {
	var appsResponse AppsResponse
	var statsResponse StatsResponse

	cfResponse := cf.Cf("curl", fmt.Sprintf("/v2/apps?q=name:%s", appName)).Wait(DEFAULT_TIMEOUT).Out.Contents()
	err := json.Unmarshal(cfResponse, &appsResponse)
	Expect(err).NotTo(HaveOccurred())
	serverAppUrl := appsResponse.Resources[0].Metadata.Url

	cfResponse = cf.Cf("curl", fmt.Sprintf("%s/stats", serverAppUrl)).Wait(DEFAULT_TIMEOUT).Out.Contents()
	err = json.Unmarshal(cfResponse, &statsResponse)
	Expect(err).NotTo(HaveOccurred())

	appIp := statsResponse["0"].Stats.Host
	appPort := fmt.Sprintf("%d", statsResponse["0"].Stats.Port)
	return appIp, appPort
}

func TestRouting(t *testing.T) {
	RegisterFailHandler(Fail)

	config = helpers.LoadConfig()

	if config.DefaultTimeout > 0 {
		DEFAULT_TIMEOUT = config.DefaultTimeout * time.Second
	}

	if config.CfPushTimeout > 0 {
		CF_PUSH_TIMEOUT = config.CfPushTimeout * time.Second
	}

	componentName := "Routing"

	rs := []Reporter{}

	context := helpers.NewContext(config)
	environment := helpers.NewEnvironment(context)

	BeforeSuite(func() {
		environment.Setup()
	})

	AfterSuite(func() {
		environment.Teardown()
	})

	if config.ArtifactsDirectory != "" {
		helpers.EnableCFTrace(config, componentName)
		rs = append(rs, helpers.NewJUnitReporter(config, componentName))
	}

	RunSpecsWithDefaultAndCustomReporters(t, componentName, rs)
}
