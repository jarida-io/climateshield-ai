<!-- SPDX-License-Identifier: Apache-2.0 -->

# Deploying the ClimateShield demo

For a **demonstration deployment carrying fictional seed data only**. The demo
population in `internal/store/seed` is entirely synthetic — invented names, a
fake `+2547000001xx` phone range — and nothing here is fit to hold real
records about real children. See "Before this could hold real data" at the end.

Run these on the host yourself. Nothing in this document should be pasted into
a chat window, an issue, or a commit.

---

## 1. Prepare the host

Debian or Ubuntu. **4 GB RAM** (on DigitalOcean, `s-2vcpu-4gb`): the peak is
the first build, which compiles eight Go binaries and bundles the dashboard.
2 GB is enough to *run* the stack but not reliably to build it — npm has been
seen to die with "Exit handler never called" on a 2 GB box. If you must use a
smaller droplet, build the images elsewhere and push them to a registry rather
than building on the host. As root:

```bash
apt-get update && apt-get install -y docker.io docker-compose-plugin git
systemctl enable --now docker
```

**Lock down SSH before anything else is exposed.** Password authentication on
a public host is the single most attacked surface you have:

```bash
ssh-copy-id root@YOUR_HOST          # from your laptop, first
# then on the host:
sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
systemctl restart ssh
```

Firewall — only HTTP, HTTPS and SSH:

```bash
ufw default deny incoming && ufw allow OpenSSH && ufw allow 80 && ufw allow 443 && ufw --force enable
```

## 2. Get the code

```bash
git clone https://github.com/jarida-io/climateshield.git
cd climateshield
```

**Check what the default branch actually contains before you deploy it.** At
the time of writing `main` is still the pre-rewrite prototype: the chain
anchor, the county briefings, the annotated scores and the current dashboard
are all on `feat/climatology-model` and are not on `main` until that branch is
merged. Deploying `main` today would deploy the old system. Confirm with
`git log --oneline -1` and check out the ref you actually mean:

```bash
git checkout feat/climatology-model   # until this is merged into main
```

Two things this stack starts that are easy to overlook when reading the base
compose file:

- **`anvil`** — a single-node EVM **development** chain (id 31337) that the
  ledger anchors each day's Merkle root to and reads back from. It is not a
  public network, it holds no value, and its history is deleted along with the
  database by `down -v`. The base file publishes it on `127.0.0.1:8545` so a
  reviewer can run `cast` against it from the host; **the production overlay
  stops publishing 8545 entirely**, and the chain stays reachable only from the
  ledger and the public API inside the compose network. The anchor and its
  verification endpoint keep working either way.
- **`briefing`** — writes the county briefings. Its default generator is a
  deterministic template; no language model runs and no credential is involved
  unless you deliberately set `BRIEFING_GENERATOR`.

## 3. The short version

Everything from here — secrets, firewall, build, seed and verification — is in
one idempotent script. Run it on the host and skip to section 6:

```bash
./scripts/deploy-droplet.sh
```

It never overwrites an existing `.env`, so re-running it after a `git pull`
updates the deployment in place and keeps the secrets generated the first time.
The sections below are what it does, for anyone who would rather do it by hand
or needs to change one step.

## 4. Generate secrets **on the host**

They are created here and never leave. Do not reuse a value you have typed
anywhere else.

```bash
cat > .env <<EOF
POSTGRES_USER=climateshield
POSTGRES_PASSWORD=$(openssl rand -hex 24)
POSTGRES_DB=climateshield
DATABASE_URL=postgres://climateshield:PLACEHOLDER@postgres:5432/climateshield?sslmode=disable
PII_KEY_HEX=$(openssl rand -hex 32)

CLIMATE_SOURCE=fixture
NOTIFY_CHANNEL=mock
PREDICTOR=rules
LOG_LEVEL=info

# A domain gives you automatic HTTPS. A bare IP cannot get a certificate,
# so use :80 and accept plain HTTP for an IP-only demo.
SITE_ADDRESS=:80
EOF

# Point DATABASE_URL at the password just generated.
sed -i "s|:PLACEHOLDER@|:$(grep '^POSTGRES_PASSWORD=' .env | cut -d= -f2)@|" .env
chmod 600 .env
```

Note what is **absent**: `PII_ALLOW_DEV_KEY`. Without it the services refuse to
start on the published placeholder key, so a deployment that skips this step
fails loudly instead of running with encryption that protects nothing.

`CLIMATE_SOURCE=fixture` keeps the demo deterministic and offline. Set
`openmeteo` for live forecasts — free, no API key, but the risk levels will
then reflect real weather rather than the documented scenario.

## 5. Deploy

```bash
docker compose -f docker-compose.yml -f deploy/docker-compose.prod.yml up -d --build
docker compose -f docker-compose.yml -f deploy/docker-compose.prod.yml ps
```

First build compiles eight Go binaries and the dashboard — several minutes.

## 6. Seed the demo population and run the pipeline

The overlay carries a one-shot `demo` service behind a profile, so the demo
runs *inside* the compose network and resolves `postgres`, `registry` and
`publicapi` by name:

```bash
docker compose -f docker-compose.yml -f deploy/docker-compose.prod.yml \
  --profile demo run --rm demo
```

Do not try to run `cmd/demo` from the host. This overlay deliberately publishes
no port for Postgres or the registry, so a host-side run cannot reach them —
and reopening those ports to seed a demonstration would undo the one security
property this overlay exists to provide.

## 7. Verify

```bash
curl -s localhost/health
curl -s localhost/v1/risk/current | head -c 300
curl -s localhost/v1/model | grep -o '"note":"[^"]*"' | head -4
curl -s localhost/v1/ledger/anchors/verify | grep -o '"status":"[a-z]*"'
```

The last one should say `verified` once the ledger has swept at least one day.
`unavailable` means the check could not run and the response says why; it is
never a fabricated match.

Then open the dashboard in a browser at your host address.

**Confirm the database is not exposed** — this should fail from your laptop:

```bash
nc -vz YOUR_HOST 5432        # expect: refused
```

If it connects, the overlay was not applied. Stop and fix it.

---

## Operating it

```bash
C="docker compose -f docker-compose.yml -f deploy/docker-compose.prod.yml"
$C logs -f publicapi        # follow a service
$C pull && $C up -d --build # update
$C down                     # stop (keeps the volume)
$C down -v                  # stop and DELETE the database
```

## What this deployment is

- **Fictional data only.** Every guardian and child is invented.
- **No SMS is sent.** `NOTIFY_CHANNEL=mock` records what it would send.
- **The public surface is aggregates only**, k≥10 suppressed, enforced by a
  contract test in CI.
- **Postgres, the registry API and the development chain are not reachable from
  the internet.** Caddy is the only public process.
- **The chain the ledger anchors to is a local development chain** started by
  this deployment. Nothing here writes to any public network, and no surface
  calls it public, immutable or decentralised.

## Before this could hold real data

Not a checklist for today — a statement of what is missing. From
[NOTES.md](../NOTES.md):

1. **Key management.** `PII_KEY_HEX` sits in a file on the host. Real
   deployments need OpenBao or equivalent, and a rotation procedure.
2. **Ledger key isolation.** Per-child HMAC keys live in a separate schema but
   the same database and role. Production needs a separate role with
   schema-scoped grants.
3. **An erasure endpoint.** `ForgetChild` is a tested library function with no
   way to invoke it. Holding real records without an operable
   right-to-erasure path is not defensible.
4. **Backups**, and a restore you have actually rehearsed.
5. **A demonstrated delivery path.** No SMS has ever been sent by this system,
   so the last hop between an alert and a guardian is unproven. See
   [docs/roadmap.md](../docs/roadmap.md).

The coverage gate used to be on this list, red at 66.8%. It now reads 90.6%
against the 80% threshold and is green, so it is not.
