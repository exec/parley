# Proxmox host watchdog (eqr)

Auto-recovery for the Proxmox host that runs the parley LXCs. The Proxmox
host itself is **not** managed by terraform — these files are installed by
hand and tracked here so they survive editor amnesia and don't drift.

## What it covers

| Failure mode                                | Recovery layer                  |
|---------------------------------------------|---------------------------------|
| ext4 `emergency_ro` on / (incident 2026-04-29) | `parley-fs-watchdog` (sysrq-b)  |
| / remounted ro for any reason               | `parley-fs-watchdog` (sysrq-b)  |
| Persistent EIO/EROFS on writes              | `parley-fs-watchdog` (sysrq-b)  |
| Full kernel hang (script can't run)         | systemd `RuntimeWatchdogSec`    |
| systemd itself dies                         | hardware watchdog (`/dev/watchdog0`) |
| LXC services bind to wrong IP at boot       | `wait-for-ip.conf` drop-ins (in `terraform/userdata-db.sh`) |

## Files

| File                          | Lives at on host                             |
|-------------------------------|----------------------------------------------|
| `parley-fs-watchdog`          | `/usr/local/sbin/parley-fs-watchdog` (0755)  |
| `parley-fs-watchdog.service`  | `/etc/systemd/system/`                       |
| `parley-fs-watchdog.timer`    | `/etc/systemd/system/`                       |
| `99-parley-sysrq.conf`        | `/etc/sysctl.d/`                             |
| `parley-hw-watchdog.conf`     | `/etc/systemd/system.conf.d/`                |

## Install / update

```bash
HOST=eqr
scp parley-fs-watchdog          $HOST:/tmp/
scp parley-fs-watchdog.service  parley-fs-watchdog.timer $HOST:/tmp/
scp 99-parley-sysrq.conf        $HOST:/tmp/
scp parley-hw-watchdog.conf     $HOST:/tmp/

ssh $HOST '
  sudo install -m 0755 /tmp/parley-fs-watchdog       /usr/local/sbin/parley-fs-watchdog
  sudo install -m 0644 /tmp/parley-fs-watchdog.{service,timer} /etc/systemd/system/
  sudo install -m 0644 /tmp/99-parley-sysrq.conf     /etc/sysctl.d/
  sudo mkdir -p /etc/systemd/system.conf.d
  sudo install -m 0644 /tmp/parley-hw-watchdog.conf  /etc/systemd/system.conf.d/

  sudo sysctl --system | grep sysrq
  sudo systemctl daemon-reload
  sudo systemctl daemon-reexec   # picks up RuntimeWatchdogSec without reboot
  sudo systemctl enable --now parley-fs-watchdog.timer
'
```

## Verifying

```bash
ssh eqr 'sudo /usr/local/sbin/parley-fs-watchdog --dry-run'   # → "[dry-run] OK"
ssh eqr 'systemctl list-timers | grep parley-fs-watchdog'      # → next-fire ≤30s
ssh eqr 'systemctl show --property=RuntimeWatchdogUSec'        # → 1min
ssh eqr 'cat /proc/sys/kernel/sysrq'                            # → 1
```

## Tunables

Edit `/usr/local/sbin/parley-fs-watchdog`:

- `MAX_REBOOTS_PER_HOUR=3` — guardrail. After this many reboots in an hour
  the watchdog logs `crit` and stops. This is the only thing protecting
  against a real hardware failure looping the box forever.
- Check cadence is in `parley-fs-watchdog.timer` (`OnUnitActiveSec=30s`).

## When it trips

Journal shows:
```
parley-fs-watchdog: TRIPPED: <reason>. Hard-resetting via sysrq-b. recent_reboots=N
```

If it tripped 3+ times in an hour:
```
parley-fs-watchdog: TRIPPED (<reason>) but N reboots in last hour — refusing further reboots. Manual intervention required.
```

In the manual-intervention case: SSH to the host, run `sudo dmesg | tail -200`
and `sudo smartctl -a /dev/nvme0n1` to characterize the failure. If the drive
is genuinely failing, replace it; if the kernel/driver looks transient, a
fresh reboot may clear it and the counter resets after an hour.
