# beads3d Performance Baselines

Generated: 2026-02-23T15:59:20Z
Environment: Playwright Chromium (SwiftShader), viewport 1280x800
Warmup: 4000ms, Measurement: 5000ms

## Results

| Nodes | Links | Avg FPS | Avg ms | p50 ms | p95 ms | p99 ms | Min ms | Max ms | Heap MB | Frames |
|------:|------:|--------:|-------:|-------:|-------:|-------:|-------:|-------:|--------:|-------:|
| 100 | 0 | 7.5 | 132.74 | 117.7 | 254.3 | 274.4 | 88.1 | 274.4 | 16.3 | 37 |
| 500 | 0 | 8.3 | 119.79 | 117.5 | 155.7 | 180.3 | 85.4 | 180.3 | 15.4 | 41 |
| 1000 | 0 | 11 | 91.01 | 87.2 | 127 | 144.5 | 74 | 144.5 | 16.3 | 54 |
| 5000 | 0 | 8.6 | 115.84 | 97.3 | 169 | 514.2 | 75.8 | 514.2 | 15.4 | 42 |

## Interpretation

- **Target**: 30+ FPS at 500 nodes, 15+ FPS at 1000 nodes
- **p95**: Frame time below 33ms means smooth at 30fps
- **p99**: Occasional spikes above 50ms are acceptable (GC, layout settling)
- **SwiftShader**: Software renderer — real GPU will be 2-4x faster
- **Heap**: Watch for linear growth with node count (indicates memory leak)

