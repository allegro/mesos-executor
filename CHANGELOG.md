# Change Log
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [0.8.1]
### Changed
- Fixed vip name retention after LBaaS registration.

## [0.8.0]
### Changed
- Call termination hooks if task exits.
- Externalize Sentry configuration.
- Remove PKI support.
### Added
- Consul ACL Support
- Optional asynchronous VaaS registration.

## [0.7.7]
### Changed
- Build using Go 1.9
### Added
- Log reason of killing.

## [0.7.6]
### Changed
- Use `VAAS_INITAL_WEIGHT` env to override initial VaaS weight.
### Added
- Add canary tag when registering canary instance in VaaS.

## [0.7.5]
### Changed
- Use VaaS ratio from weight label/tag not from env.
### Fixed
- Enable LBaaS hook.
- Call consul hook after other hooks.

## [0.7.4]
### Changed
- Use LBaaS ratio from weight label/tag. If not found do NOT set it to default.

## [0.7.3]
### Changed
- Removed unintentional dependency on supervisor code

## [0.7.2]
### Fixed
- Filter VIPs for service additionally by environment

## [0.7.0]
### Changed
- Disable LBaaS registration
### Added
- Schedule task kill when validate-certificate label is set to true
- Handle TaskStatus acknowledge
### Fixed
- SHA-256 is used, instead of a deprecated SHA-1, for the certificate 
request signing

## [0.6.0]
### Added
- Enabled Synchronous Consul registration
- LBaaS registration hook
- Read VaaS configuration from environment
- Migration to AfterTaskHealthyEvent hook

## [0.5.1]
### Added
- Integration with Vault PKI
- Synchronous Consul registration hook (disabled)
### Fixed
- Pass hook errors in update message
- Handle all events in one place

## [0.5.0]
### Added
- HTTP/TCP custom executor health check support

## [0.4.2]
### Added
- Send AppEngine environment with Sentry alerts

## [0.4.1]
### Changed
- Send only fatal level logs to Sentry

## [0.4.0]
### Added
- Command healthcheck support
- More generic interface for hooks
- Support for configuration via environment variables
- VaaS registration hooks with weight support
### Fixed
- Better error handling for kill/term signalling
- Don't crash when not using vaas hook

## [0.3.1]
### Added
- Support for TASK_KILLING state
- Send detailed messages to Mesos agent with important state updates
### Fixed
- Send buffered state updates before finishing executor

## [0.3.0]
### Added
- Consul deregistration hook

## [0.2.1]
### Changed
- Send SIGTERM/SIGKILL signals to the pgid instead of a pid

## [0.2.0]
### Changed
- Migration to v1 Mesos HTTP API

## [0.1.4]
### Fixed
- Build statically linked executor
