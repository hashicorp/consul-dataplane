# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# These scan results are run as part of CRT workflows.

# Un-triaged results will block release. See `security-scanner` docs for more
# information on how to add `triage` config to unblock releases for specific results.
# In most cases, we should not need to disable the entire scanner to unblock a release.

# To run manually, install scanner and then from the repository root run
# `SECURITY_SCANNER_CONFIG_FILE=.release/security-scan.hcl scan ...`
# To scan a local container, add `local_daemon = true` to the `container` block below.
# See `security-scanner` docs or run with `--help` for scan target syntax.

container {
  dependencies    = true
  alpine_security = true
  osv             = true
  go_modules      = true

  secrets {
    all = true
  }

  # Triage items that are _safe_ to ignore here. Note that this list should be
  # periodically cleaned up to remove items that are no longer found by the scanner.
  triage {
    suppress {
      vulnerabilities = [
        "CVE-2025-6965",
        "CVE-2025-6395",
        "CVE-2024-12797",
        "CVE-2025-5702",
        "CVE-2025-8058",
        "CVE-2024-4067",
        "CVE-2025-31115",
        "CVE-2025-3576",
        "CVE-2025-6021",
        "CVE-2025-25724",
        "CVE-2024-57970",
        "CVE-2025-32414",
        "CVE-2024-52533",
        "CVE-2025-5914",
        "CVE-2025-3277",
        "CVE-2024-40896",
        #  Dependency Scanner
        "DLA-3972-1", # var/lib/dpkg/status.d/tzdata:
        "DLA-4085-1",
        "DLA-4105-1",
        "DLA-4403-1",
        "DEBIAN-CVE-2023-5678", # var/lib/dpkg/status.d/openssl:
        "DEBIAN-CVE-2024-0727",
        "DEBIAN-CVE-2024-2511",
        "DEBIAN-CVE-2024-4741",
        "DEBIAN-CVE-2024-5535",
        "DEBIAN-CVE-2024-9143",
        "DEBIAN-CVE-2024-13176",
        "DEBIAN-CVE-2025-9230",
        "DEBIAN-CVE-2025-27587",
        "DLA-3942-2",
        "DLA-4176-1",
        "DLA-4321-1"
        # Go Modules Scanner usr/local/bin/discover
        "GHSA-4f99-4q7p-p3gh",
        "GO-2025-4116",
        "GO-2025-4134",
        "GO-2025-4135",
        "GO-2025-4188",
        "GHSA-f6x5-jh6r-wrfv",
        "GHSA-j5w8-q4qc-rx2x",
      ]
    }
  }
}

binary {
  go_modules = true
  osv        = true

  secrets {
    all = true
  }
}

repository {
  go_modules = true
  npm        = true
  osv        = true

  secrets {
    all = true
  }

  triage {
    suppress {
      # Only remaining vulnerabilities in integration tests (archived go-jose v2)
      vulnerabilities = [
        "CVE-2024-28180",
        "GHSA-c5q2-7r4c-mv6g",
        "GO-2024-2631"
      ]
      paths = [
        # SHA1 usage in bootstrap config is for non-security purposes
        "internal/bootstrap/bootstrap_config.go"
      ]
    }
  }
}
