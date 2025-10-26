package client

const (
	// ENV_PROD will talk to the ICANN MOSAPI production API
	ENV_PROD = "prod"

	// ENV_OTE will talk to the ICANN MOSAPI OTE API
	ENV_OTE = "ote"

	// AUTH_TYPE_TLSA is the auth type for certificate based authentication
	AUTH_TYPE_TLSA = "tlsa"

	// AUTH_TYPE_BASIC is the auth type for basic authentication
	AUTH_TYPE_BASIC = "basic"

	MOSAPI_URL     = "https://mosapi.icann.org"
	MOSAPI_OTE_URL = "https://mosapi-ote.icann.org"

	ServiceEPP    = "EPP"
	ServiceDNS    = "DNS"
	ServiceDNSSEC = "DNSSEC"
	ServiceRDDS   = "RDDS"

	EntityRegistry  = "ry"
	EntityRegistrar = "rr"

	V2 = "v2"
)

var (
	// validEnvs is a list of valid environments we accept
	validEnvs = []string{ENV_PROD, ENV_OTE}

	// validAuthTypes is a list of valid authentication types we accept
	validAuthTypes = []string{AUTH_TYPE_TLSA, AUTH_TYPE_BASIC}

	// validServices is a list of valid services we accept
	validServices = []string{ServiceEPP, ServiceDNS, ServiceDNSSEC, ServiceRDDS}

	// validEntities is a list of valid entities we accept
	validEntities = []string{EntityRegistry, EntityRegistrar}

	// validVersions is a list of valid versions we accept
	validVersions = []string{V2}
)
