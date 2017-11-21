# Change Log
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [0.8.1]
### Changed
- [DEPLOY-676] Fixed vip name retention after LBaaS registration.

## [0.8.0]
### Changed
- [DEPLOY-676] Call termination hooks if task exits.
- [DEPLOY-667] Externalize Sentry configuration.
- [DEPLOY-663] Remove PKI support.
### Added
- [SKYLAB-2039] Consul ACL Support
- [DEPLOY-670] Optional asynchronous VaaS registration.

## [0.7.7]
### Changed
- [DEPLOY-573] Build using Go 1.9
### Added
- [DEPLOY-587] Log reason of killing.

## [0.7.6]
### Changed
- [DEPLOY-405] Use `VAAS_INITAL_WEIGHT` env to override initial VaaS weight.
### Added
- [DEPLOY-447] Add canary tag when registering canary instance in VaaS.

## [0.7.5]
### Changed
- [DEPLOY-405] Use VaaS ratio from weight label/tag not from env.
### Fixed
- [DEPLOY-421] Enable LBaaS hook.
- [DEPLOY-421] Call consul hook after other hooks.

## [0.7.4]
### Changed
- [DEPLOY-405] Use LBaaS ratio from weight label/tag. If not found do NOT set it to default.

## [0.7.3]
### Changed
- [DEPLOY-403] Removed unintentional dependency on supervisor code

## [0.7.2]
### Fixed
- [DEPLOY-401] Filter VIPs for service additionally by environment

## [0.7.0]
### Changed
- Disable LBaaS registration
### Added
- [DEPLOY-328] Schedule task kill when validate-certificate label is set to true
- [DEPLOY-307] Handle TaskStatus acknowledge
### Fixed
- [DEPLOY-341] SHA-256 is used, instead of a deprecated SHA-1, for the certificate 
request signing

## [0.6.0]
### Added
- [DEPLOY-262] Enabled Synchronous Consul registration
- [DEPLOY-252] LBaaS registration hook
- [DEPLOY-180] Read VaaS configuration from environment
- [DEPLOY-240] Migration to AfterTaskHealthyEvent hook

## [0.5.1]
### Added
- [DEPLOY-4] Integration with Vault PKI
- [DEPLOY-182] Synchronous Consul registration hook (disabled)
### Fixed
- [DEPLOY-244] Pass hook errors in update message
- [DEPLOY-187] Handle all events in one place

## [0.5.0]
### Added
- [DEPLOY-7] HTTP/TCP custom executor health check support

## [0.4.2]
### Added
- [DEPLOY-230] Send AppEngine environment with Sentry alerts

## [0.4.1]
### Changed
- [DEPLOY-230] Send only fatal level logs to Sentry

## [0.4.0]
### Added
- [DEPLOY-7] Command healthcheck support
- [DEPLOY-4] More generic interface for hooks
- [DEPLOY-11] Support for configuration via environment variables
- [DEPLOY-5] VaaS registration hooks with weight support
### Fixed
- [DEPLOY-150] Better error handling for kill/term signalling
- [DEPLOY-183] Don't crash when not using vaas hook

## [0.3.1]
### Added
- [DEPLOY-156] Support for TASK_KILLING state
- [DEPLOY-164] Send detailed messages to Mesos agent with important state updates
### Fixed
- [DEPLOY-149] Send buffered state updates before finishing executor

## [0.3.0]
### Added
- [DEPLOY-23] Consul deregistration hook

## [0.2.1]
### Changed
- [DEPLOY-1] Send SIGTERM/SIGKILL signals to the pgid instead of a pid

## [0.2.0]
### Changed
- [PERFORCE-69] Migration to v1 Mesos HTTP API

## [0.1.4]
### Fixed
- [PERFORCE-66] Build statically linked executor
