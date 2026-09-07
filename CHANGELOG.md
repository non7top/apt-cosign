# Changelog

## [0.8.0](https://github.com/non7top/apt-cosign/compare/v0.7.0...v0.8.0) (2026-09-07)


### Features

* add README and CI-built release workflow ([51b60fb](https://github.com/non7top/apt-cosign/commit/51b60fbc97345a08ee609aff1db8cd0585c466e4))
* adopt release-please for versioning and releases ([9a0d186](https://github.com/non7top/apt-cosign/commit/9a0d1865df77701a056b42ccba13d9b179b9e47e))
* **ci:** extract apt-repo publish/sign into reusable composite actions ([f36ddb0](https://github.com/non7top/apt-cosign/commit/f36ddb0f928700f48e79544942ed5fcbfa5ec3aa))
* **ci:** extract apt-repo publish/sign into reusable composite actions ([5a9f9d3](https://github.com/non7top/apt-cosign/commit/5a9f9d39fba0f2ef6f5280e07afd594e4f97e2fb))
* derive owner/repo for zero-config GitHub-hosted sources ([2d19395](https://github.com/non7top/apt-cosign/commit/2d193956ef96788d25c6f79941dff835ab6ec638))
* derive owner/repo for zero-config GitHub-hosted sources ([7201447](https://github.com/non7top/apt-cosign/commit/72014476c9feeb8c455b521ef99ff885b5471c7a))
* host apt-cosign as a real raw.githubusercontent.com apt repo ([0d749ee](https://github.com/non7top/apt-cosign/commit/0d749ee8747b7b62964c29545b733192fdbabbe8))
* host apt-cosign as a real raw.githubusercontent.com apt repo ([bd1250e](https://github.com/non7top/apt-cosign/commit/bd1250e1bbbcef0c4829d1743f28e7e2bedd45aa))
* per-source identity policy, derived owner/repo, self-update default ([b07c028](https://github.com/non7top/apt-cosign/commit/b07c028ed55a97dc07257c76d39dae9818493b55))
* per-source identity policy, derived owner/repo, self-update default ([1becdf3](https://github.com/non7top/apt-cosign/commit/1becdf33767112d7c62d020dfe6787333aa2d4a0))
* stage and sign a Packages.gz alongside the plain Packages index ([26ea250](https://github.com/non7top/apt-cosign/commit/26ea25099c6788055c2dc0c514de9935d321ac37))
* stage and sign a Packages.gz alongside the plain Packages index ([54d56ef](https://github.com/non7top/apt-cosign/commit/54d56ef4f0a19c21b238e013991f9beac73ddbc5)), closes [#21](https://github.com/non7top/apt-cosign/issues/21)


### Bug Fixes

* **debian:** stop shipping the example policy where apt silently ignores it ([d71fcca](https://github.com/non7top/apt-cosign/commit/d71fcca31b8a103c188a89cdb830bc69520462fd))
* **debian:** stop shipping the example policy where apt silently ignores it ([9253ffd](https://github.com/non7top/apt-cosign/commit/9253ffdde7038d4661eca15f591328c67afa2c30))
* drop the shipped self-update default policy ([ebff25b](https://github.com/non7top/apt-cosign/commit/ebff25b5733e0c0ba7024dd49d978387bf7e0563))

## [0.7.0](https://github.com/non7top/apt-cosign/compare/v0.6.0...v0.7.0) (2026-09-07)


### Features

* stage and sign a Packages.gz alongside the plain Packages index ([e823892](https://github.com/non7top/apt-cosign/commit/e82389271a35544d102022b360f8556218fb431d))
* stage and sign a Packages.gz alongside the plain Packages index ([b503c79](https://github.com/non7top/apt-cosign/commit/b503c7967c81cf8fe425bfba8e0ef425af00d5b3)), closes [#21](https://github.com/non7top/apt-cosign/issues/21)


### Bug Fixes

* **debian:** stop shipping the example policy where apt silently ignores it ([6dc78d7](https://github.com/non7top/apt-cosign/commit/6dc78d7343d149fa15fcd2d617c0f48fe3c9ab7f))
* **debian:** stop shipping the example policy where apt silently ignores it ([4f71a66](https://github.com/non7top/apt-cosign/commit/4f71a666da1db374a6f7e370388ca7c0e14b31bb))

## [0.6.0](https://github.com/non7top/apt-cosign/compare/v0.5.0...v0.6.0) (2026-09-06)


### Features

* **ci:** extract apt-repo publish/sign into reusable composite actions ([637564c](https://github.com/non7top/apt-cosign/commit/637564cade48dd116775fc1a98e68b669f76d9cc))
* **ci:** extract apt-repo publish/sign into reusable composite actions ([380196e](https://github.com/non7top/apt-cosign/commit/380196e2f09ea83e933112e0ae21888301e0ec2f))

## [0.5.0](https://github.com/non7top/apt-cosign/compare/v0.4.0...v0.5.0) (2026-09-06)


### Features

* derive owner/repo for zero-config GitHub-hosted sources ([671528c](https://github.com/non7top/apt-cosign/commit/671528c9a764fef4830faccf1000a288e10901d5))
* derive owner/repo for zero-config GitHub-hosted sources ([d2a6cc1](https://github.com/non7top/apt-cosign/commit/d2a6cc1cf38844c6d6a4342e40d64ebd517293f4))

## [0.4.0](https://github.com/non7top/apt-cosign/compare/v0.3.0...v0.4.0) (2026-09-06)


### Features

* per-source identity policy, derived owner/repo, self-update default ([b80b24d](https://github.com/non7top/apt-cosign/commit/b80b24d33d9cd44a893e8ebed2e79c01853ce344))
* per-source identity policy, derived owner/repo, self-update default ([f193cff](https://github.com/non7top/apt-cosign/commit/f193cffc0d43ea2d59cbc8017227f7d7091fe0cb))


### Bug Fixes

* drop the shipped self-update default policy ([a5fafe5](https://github.com/non7top/apt-cosign/commit/a5fafe5986d6d915ee226e2a87f1b1a3ddd08ead))

## [0.3.0](https://github.com/non7top/apt-cosign/compare/v0.2.0...v0.3.0) (2026-09-06)


### Features

* host apt-cosign as a real raw.githubusercontent.com apt repo ([2c7bfce](https://github.com/non7top/apt-cosign/commit/2c7bfce38b2a178da75777431a0615ff1db56ad8))
* host apt-cosign as a real raw.githubusercontent.com apt repo ([6171a18](https://github.com/non7top/apt-cosign/commit/6171a18473172c7537111fd7a9512246664acbcd))

## [0.2.0](https://github.com/non7top/apt-cosign/compare/v0.1.0...v0.2.0) (2026-09-06)


### Features

* add README and CI-built release workflow ([b9f89a7](https://github.com/non7top/apt-cosign/commit/b9f89a73b67305901f5128c981a840e813174c1e))
* adopt release-please for versioning and releases ([915b432](https://github.com/non7top/apt-cosign/commit/915b432904f623412804dcfe58a2e92f3401520c))
