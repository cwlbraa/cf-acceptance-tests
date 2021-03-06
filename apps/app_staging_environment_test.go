package apps

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	. "github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/app_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
	archive_helpers "github.com/pivotal-golang/archiver/extractor/test_helper"
)

var _ = Describe("Buildpack Environment", func() {
	var (
		appName       string
		BuildpackName string

		appPath string

		buildpackPath        string
		buildpackArchivePath string

		tmpdir string
	)

	matchingFilename := func(appName string) string {
		return fmt.Sprintf("buildpack-environment-match-%s", appName)
	}

	BeforeEach(func() {
		cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
			BuildpackName = RandomName()
			appName = PrefixedRandomName("CATS-APP-")

			var err error
			tmpdir, err = ioutil.TempDir("", "buildpack_env")
			Expect(err).ToNot(HaveOccurred())
			appPath, err = ioutil.TempDir(tmpdir, "matching-app")
			Expect(err).ToNot(HaveOccurred())

			buildpackPath, err = ioutil.TempDir(tmpdir, "matching-buildpack")
			Expect(err).ToNot(HaveOccurred())

			buildpackArchivePath = path.Join(buildpackPath, "buildpack.zip")

			archive_helpers.CreateZipArchive(buildpackArchivePath, []archive_helpers.ArchiveFile{
				{
					Name: "bin/compile",
					Body: `#!/usr/bin/env bash

sleep 5

echo RUBY_LOCATION=$(which ruby)
echo RUBY_VERSION=$(ruby --version)

sleep 10
`,
				},
				{
					Name: "bin/detect",
					Body: fmt.Sprintf(`#!/bin/bash

if [ -f "${1}/%s" ]; then
  echo Simple
else
  echo no
  exit 1
fi
`, matchingFilename(appName)),
				},
				{
					Name: "bin/release",
					Body: `#!/usr/bin/env bash

cat <<EOF
---
config_vars:
  PATH: bin:/usr/local/bin:/usr/bin:/bin
  FROM_BUILD_PACK: "yes"
default_process_types:
  web: while true; do { echo -e 'HTTP/1.1 200 OK\r\n'; echo "hi from a simple admin buildpack"; } | nc -l \$PORT; done
EOF
`,
				},
			})
			_, err = os.Create(path.Join(appPath, matchingFilename(appName)))
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Create(path.Join(appPath, "some-file"))
			Expect(err).ToNot(HaveOccurred())

			createBuildpack := cf.Cf("create-buildpack", BuildpackName, buildpackArchivePath, "0").Wait(DEFAULT_TIMEOUT)
			Expect(createBuildpack).Should(Exit(0))
			Expect(createBuildpack).Should(Say("Creating"))
			Expect(createBuildpack).Should(Say("OK"))
			Expect(createBuildpack).Should(Say("Uploading"))
			Expect(createBuildpack).Should(Say("OK"))
		})
	})

	AfterEach(func() {
		app_helpers.AppReport(appName, DEFAULT_TIMEOUT)

		Expect(cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))

		cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
			Expect(cf.Cf("delete-buildpack", BuildpackName, "-f").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		})

		os.RemoveAll(tmpdir)
	})

	It("uses ruby 1.9.3-p547 for staging", func() {
		Expect(cf.Cf("push", appName,
			"--no-start",
			"-b", BuildpackName,
			"-m", DEFAULT_MEMORY_LIMIT,
			"-p", appPath,
			"-d", config.AppsDomain,
		).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		app_helpers.SetBackend(appName)

		start := cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)
		Expect(start).To(Exit(0))
		Expect(start).To(Say("RUBY_LOCATION=/usr/bin/ruby"))
		Expect(start).To(Say("RUBY_VERSION=ruby 1.9.3p547"))
	})
})
