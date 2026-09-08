# Container sandbox profiles

`seccomp.json` is derived from Moby's default Docker profile at commit
`61eaf32614c7c71b60bd8927d3e6a4ffc8ff1f31`:
https://github.com/moby/profiles/blob/61eaf32614c7c71b60bd8927d3e6a4ffc8ff1f31/seccomp/default.json

The upstream Apache 2.0 license is included as `LICENSE.moby`.

The only behavioral change adds an allow rule for `clone`, `unshare`, `setns`,
`mount`, `umount2`, and `pivot_root`. This lets non-root Codex create its inner
user, mount, and PID namespaces. Docker's other default rules remain; `clone3`
still returns the upstream ENOSYS fallback without CAP_SYS_ADMIN. JSON indentation
is normalized to two spaces.

`apparmor.profile` derives from Moby v28.0.0's
[default AppArmor template](https://github.com/moby/moby/blob/v28.0.0/profiles/apparmor/template.go),
under the same Apache 2.0 license. It retains the default proc/sys denials,
same-profile ptrace rule and runtime signal reception. It replaces `deny mount`
with mount/pivot-root permissions and explicitly allows user namespaces for
AppArmor 4. The fixed name is `magent-container-v1`. This permits Codex/bubblewrap's
inner namespace construction; it does not grant CAP_SYS_ADMIN to the outer
container. `scripts/magent-apparmor.sh` explicitly loads and persists only that
named profile on a Linux AppArmor host. It requires AppArmor 4 or newer and root
for that one host setup operation. Launchers select it when Docker reports
AppArmor, and never install it automatically. Docker Desktop hosts without
AppArmor continue using the existing seccomp policy.

This expands the kernel operations available to container processes. The
launchers compensate by dropping all host capabilities and enabling
`no-new-privileges`; they do not enable privileged mode, mount the Docker socket,
disable host AppArmor, change host sysctls, or disable Codex's read-only/workspace-write enforcement.
The profile is applied to this container only. It is not a replacement for
provider sandbox checks or a guarantee of support on every Docker host.

When updating the profile or pinned Codex runtime, run the offline smoke tests
that demonstrate both permitted workspace edits and denied read-only edits.
Additional host restrictions (for example a stricter administrator policy) may
still prevent sandbox startup. Report those failures rather than automatically
relaxing the host's security policy.
