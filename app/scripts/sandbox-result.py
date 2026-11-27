#!/usr/bin/env python3
"""Simulate a Sandbox review; credentials come only from the environment."""
import argparse
import hashlib
import hmac
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("applicant_id", help="Sandbox applicant ID copied from Dashboard")
    parser.add_argument("result", choices=["GREEN", "RETRY", "FINAL"])
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{24}", args.applicant_id):
        parser.error("Expected a 24-character applicant ID")
    token = os.environ.get("SUMSUB_APP_TOKEN", "")
    secret = os.environ.get("SUMSUB_SECRET_KEY", "")
    if not token or not secret:
        parser.error("SUMSUB_APP_TOKEN and SUMSUB_SECRET_KEY are required")
    # This endpoint itself rejects Production; no configurable host or redirect.
    payload = {"reviewAnswer": "GREEN", "rejectLabels": []}
    if args.result != "GREEN":
        payload = {"reviewAnswer": "RED", "reviewRejectType": args.result,
                   "rejectLabels": ["UNSATISFACTORY_PHOTOS" if args.result == "RETRY" else "FORGERY"]}
    body = json.dumps(payload, separators=(",", ":")).encode()
    path = f"/resources/applicants/{args.applicant_id}/status/testCompleted"
    timestamp = str(int(time.time()))
    signature = hmac.new(secret.encode(), (timestamp + "POST" + path).encode() + body,
                         hashlib.sha256).hexdigest()
    request = urllib.request.Request("https://api.sumsub.com" + path, data=body,
        headers={"Content-Type": "application/json", "X-App-Token": token,
                 "X-App-Access-Ts": timestamp, "X-App-Access-Sig": signature}, method="POST")
    try:
        with urllib.request.build_opener(NoRedirect).open(request, timeout=30) as response:
            print(f"testCompleted {args.result}: HTTP {response.status}")
    except urllib.error.HTTPError as error:
        print(f"testCompleted failed: HTTP {error.code}; response body withheld", file=sys.stderr)
        return 1
    except (urllib.error.URLError, TimeoutError):
        print("testCompleted network error; details withheld", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
