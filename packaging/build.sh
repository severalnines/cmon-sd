#!/usr/bin/env bash

dir=$(cd "$(dirname "$0")" && pwd)
repoRoot=$(dirname "$dir")

mainVersion=$(cat "${dir}/version")
buildVersion=$1

if [ "${buildVersion}" == "" ]; then
    buildVersion="dev"
fi

echo "Building ${mainVersion}-${buildVersion}"

# Create build dir (at repo root — mounted as /root/package/build in Jenkins)
[ ! -d "${repoRoot}/build" ] && mkdir -p "${repoRoot}/build"

# Build Go binary from repo root
cd "${repoRoot}"
CGO_ENABLED=0 \
GOOS=linux \
go build \
  -a \
  -buildvcs=false \
  -ldflags "-s -w -X main.buildVersion=${buildVersion} -X main.mainVersion=${mainVersion} -extldflags \"-static\"" \
  -tags "osusergo netgo" \
  -o "${dir}/cmon_sd" \
  .
[ $? != "0" ] && echo "go build failed" && exit 1

# Create deb package
cd "${dir}"
rm -rf _CPack_Packages CMakeCache.txt
cmake . \
    -DCMAKE_INSTALL_PREFIX=/usr/share/cmon-sd \
    -DBUILDNUM=${buildVersion} \
    -DCPACK_GENERATOR=DEB
[ $? != "0" ] && echo "cmake deb build failed" && exit 1
make package
[ $? != "0" ] && echo "make deb failed" && exit 1

# Create rpm package
rm -rf _CPack_Packages CMakeCache.txt
cmake . \
    -DCMAKE_INSTALL_PREFIX=/usr/share/cmon-sd \
    -DBUILDNUM=${buildVersion} \
    -DCPACK_GENERATOR=RPM
[ $? != "0" ] && echo "cmake rpm build failed" && exit 1
make package
[ $? != "0" ] && echo "make rpm failed" && exit 1

# Move packages to repo root build/ (= $PACKAGES_DIR mount in Jenkins)
[ -f "${dir}/clustercontrol-sd_${mainVersion}-${buildVersion}_x86_64.deb" ] && mv "${dir}/clustercontrol-sd_${mainVersion}-${buildVersion}_x86_64.deb" "${repoRoot}/build/"
[ -f "${dir}/clustercontrol-sd-${mainVersion}-${buildVersion}-x86_64.rpm" ] && mv "${dir}/clustercontrol-sd-${mainVersion}-${buildVersion}-x86_64.rpm" "${repoRoot}/build/"

# Clean
rm -rf \
    "${dir}/_CPack_Packages" \
    "${dir}/CMakeFiles" \
    "${dir}/CMakeCache.txt" \
    "${dir}/cmake_install.cmake" \
    "${dir}/CPackConfig.cmake" \
    "${dir}/CPackSourceConfig.cmake" \
    "${dir}/install_manifest.txt" \
    "${dir}/Makefile" \
    "${dir}/cmon_sd"
