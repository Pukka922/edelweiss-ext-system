export interface Info {
  hostname: string;
  os: { name: string; version: string; prettyName: string };
  kernel: string;
  arch: string;
  cpu: { model: string; cores: number };
  memoryTotal: number;
  bootTime: string;
  uptimeSeconds: number;
}

export interface Disk {
  mount: string;
  device: string;
  fsType: string;
  total: number;
  used: number;
}

export interface NetRate {
  iface: string;
  rxBps: number;
  txBps: number;
}

export interface Metrics {
  time: string;
  cpu: number;
  load: [number, number, number];
  memory: { total: number; available: number; used: number; swapTotal: number; swapUsed: number };
  disks: Disk[];
  net: NetRate[];
}
