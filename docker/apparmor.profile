# Derived from Moby v28.0.0 profiles/apparmor/template.go (Apache 2.0).
# See ../README.md#third-party-notices. Only containers selecting this name use this policy.
#include <tunables/global>

profile magent-container-v1 flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  network,
  capability,
  file,
  umount,

  signal (receive) peer=unconfined,
  signal (receive) peer=runc,
  signal (receive) peer=crun,
  signal (send,receive) peer=magent-container-v1,

  # Codex/bubblewrap creates its own user and mount namespaces. The launcher
  # drops ALL capabilities, so these rules do not grant mounts in the outer
  # container namespace. AppArmor 4 explicitly mediates user namespace creation.
  userns,
  mount,
  pivot_root,

  # Preserve Docker's default proc/sys write and kernel-interface restrictions.
  deny @{PROC}/* w,
  deny @{PROC}/{[^1-9],[^1-9][^0-9],[^1-9s][^0-9y][^0-9s],[^1-9][^0-9][^0-9/]*}/** w,
  deny @{PROC}/sys/[^k]** w,
  deny @{PROC}/sys/kernel/{?,??,[^s][^h][^m]**} w,
  deny @{PROC}/sysrq-trigger rwklx,
  deny @{PROC}/kcore rwklx,
  deny /sys/[^f]*/** wklx,
  deny /sys/f[^s]*/** wklx,
  deny /sys/fs/[^c]*/** wklx,
  deny /sys/fs/c[^g]*/** wklx,
  deny /sys/fs/cg[^r]*/** wklx,
  deny /sys/firmware/** rwklx,
  deny /sys/devices/virtual/powercap/** rwklx,
  deny /sys/kernel/security/** rwklx,

  ptrace (trace,read,tracedby,readby) peer=magent-container-v1,
}
