# Human TODO for Launch: LogSieve

**The core deduplication engine (Drain3, Context Window, Fingerprinting) is complete and highly performant.**

LogSieve represents your most lucrative enterprise opportunity. You are not hosting a SaaS—you are selling a licensed Docker Image/Binary that companies run in *their own* AWS/GCP clusters to save money on Datadog.

## 1. Implement License Key Validation (The Only Code Left)
To make this a self-serve commercial product, you must gate the binary.
- [ ] Sign up for **LemonSqueezy** or **Keygen.sh**.
- [ ] Add a lightweight Go middleware to `cmd/server/main.go` or `pkg/config/loader.go` that reads a `LOGSIEVE_LICENSE_KEY` environment variable.
- [ ] Have the Go binary ping the LemonSqueezy/Keygen API once on startup (and maybe once every 24 hours in a goroutine) to validate the license. If invalid, the server fatally exits.

## 2. Infrastructure & Packaging
- [ ] Run `make docker-build` to package the final binary.
- [ ] Push the Docker image to a registry (e.g., Docker Hub or GitHub Container Registry). You can make the image public, because without the `LOGSIEVE_LICENSE_KEY`, it won't run.

## 3. Marketing Site (The ROI Calculator)
- [ ] Create a simple landing page (Next.js, Webflow, or Framer).
- [ ] **Critical:** Build an interactive "ROI Calculator". 
  - *Slider:* "How many GBs of logs do you generate per month?"
  - *Slider:* "What is your Datadog/Splunk cost per GB?"
  - *Result:* "LogSieve will save you $X,XXX per month by cutting log volume by 80%."
- [ ] Connect a LemonSqueezy checkout button (e.g., $49/mo or $299 lifetime license) that automatically issues the License Key.

## 4. The "Show, Don't Tell" Launch Assets
- [ ] **Record a 60-second Loom Demo:**
  - Show a terminal tailing a massive, noisy log stream.
  - Show Grafana/Loki hitting thousands of logs a minute.
  - Turn on LogSieve.
  - Show the Grafana ingestion rate plummet by 90%, while preserving the actual errors.
- [ ] **Take 2-3 Screenshots:**
  - A Grafana dashboard showing the `logsieve_dedup_ratio` metric sitting at 92%.
  - The clean Docker compose file proving it's just a drop-in sidecar.

## 5. Community Launch
- [ ] Post your Loom video to r/DevOps, r/Kubernetes, and r/SRE. 
- [ ] Launch on Hacker News with the hook: *"Datadog's billing is out of control, so I built a self-hosted sidecar that deduplicates container logs by 90% before they ever leave your cluster."*