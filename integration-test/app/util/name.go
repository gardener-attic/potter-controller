package util

func CreateEchoServerInstallationNameWithSuffix(config *IntegrationTestConfig, suffix string) string {
	if config.TestLandscaper {
		return config.TestPrefix + "-ls-echo-" + suffix
	}

	return config.TestPrefix + "-echo-" + suffix
}

func CreateEchoServerInstallationName(config *IntegrationTestConfig) string {
	if config.TestLandscaper {
		return config.TestPrefix + "-ls-echo"
	}

	return config.TestPrefix + "-echo"
}

func CreateJobNameWithSuffix(config *IntegrationTestConfig, suffix string) string {
	return config.TestPrefix + "-job-" + suffix
}
