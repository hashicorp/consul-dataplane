schema = 1
artifacts {
  zip = [
    "consul-dataplane_${version}+fips1402_linux_amd64.zip",
    "consul-dataplane_${version}+fips1402_linux_arm64.zip",
    "consul-dataplane_${version}_darwin_amd64.zip",
    "consul-dataplane_${version}_darwin_arm64.zip",
    "consul-dataplane_${version}_linux_386.zip",
    "consul-dataplane_${version}_linux_amd64.zip",
    "consul-dataplane_${version}_linux_arm.zip",
    "consul-dataplane_${version}_linux_arm64.zip",
  ]
  rpm = [
    "consul-dataplane-${version_linux}-1.aarch64.rpm",
    "consul-dataplane-${version_linux}-1.armv7hl.rpm",
    "consul-dataplane-${version_linux}-1.i386.rpm",
    "consul-dataplane-${version_linux}-1.x86_64.rpm",
    "consul-dataplane-fips-${version_linux}+fips1402-1.aarch64.rpm",
    "consul-dataplane-fips-${version_linux}+fips1402-1.x86_64.rpm",
  ]
  deb = [
    "consul-dataplane-fips_${version_linux}+fips1402-1_amd64.deb",
    "consul-dataplane-fips_${version_linux}+fips1402-1_arm64.deb",
    "consul-dataplane_${version_linux}-1_amd64.deb",
    "consul-dataplane_${version_linux}-1_arm64.deb",
    "consul-dataplane_${version_linux}-1_armhf.deb",
    "consul-dataplane_${version_linux}-1_i386.deb",
  ]
  container = [
    "consul-dataplane_release-default_linux_amd64_${version}_${commit_sha}.docker.dev.tar",
    "consul-dataplane_release-default_linux_amd64_${version}_${commit_sha}.docker.tar",
    "consul-dataplane_release-default_linux_arm64_${version}_${commit_sha}.docker.dev.tar",
    "consul-dataplane_release-default_linux_arm64_${version}_${commit_sha}.docker.tar",
    "consul-dataplane_release-fips-default_linux_amd64_${version}+fips1402_${commit_sha}.docker.dev.tar",
    "consul-dataplane_release-fips-default_linux_amd64_${version}+fips1402_${commit_sha}.docker.tar",
    "consul-dataplane_release-fips-default_linux_arm64_${version}+fips1402_${commit_sha}.docker.dev.tar",
    "consul-dataplane_release-fips-default_linux_arm64_${version}+fips1402_${commit_sha}.docker.tar",
    "consul-dataplane_release-fips-ubi_linux_amd64_${version}+fips1402_${commit_sha}.docker.dev.tar",
    "consul-dataplane_release-fips-ubi_linux_amd64_${version}+fips1402_${commit_sha}.docker.redhat.tar",
    "consul-dataplane_release-fips-ubi_linux_amd64_${version}+fips1402_${commit_sha}.docker.tar",
    "consul-dataplane_release-ubi_linux_amd64_${version}_${commit_sha}.docker.dev.tar",
    "consul-dataplane_release-ubi_linux_amd64_${version}_${commit_sha}.docker.redhat.tar",
    "consul-dataplane_release-ubi_linux_amd64_${version}_${commit_sha}.docker.tar",
  ]
}
