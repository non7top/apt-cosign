# Changelog

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
