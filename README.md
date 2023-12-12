# BtcMinerProxy

Extremely high performance Bitcoin Stratum mining protocol proxy.
BtcMinerProxy takes full advantage of device multithreading.
Reduces pool connections by up to 256 times, 1000 connected miners become just 4 for the pool.

## Download
https://github.com/venusgalstar/BtcMinerProxy/releases

## Logging
docker-compose up -d
docker-compose logs > log.txt

## Notes
- If you are using Linux and want to handle more than 1000 connections, you need to [increase the open files limit](ulimit.md)
- Miners MUST support Nicehash mode.
- BtcMinerProxy is still in beta, please report any issue.

## Donations
BtcMinerProxy has **0% fee by default**.
However, any donation is greatly appreciated!
`86Cyc69WoNa71qjStepJ83PKpR2PFEpviZJaxqT8cuvv1RLhJhf6aZAXkFA2btmHkXULyZ3bDu6uzJX2DuVkVeUwTN2M5g3`

## Comparison with other proxies

| Proxy          | Performance | Multithread | Default Fee | Cross-platform | Reduces pool load | 1-step setup |
|----------------|-------------|-------------|-------------|----------------|-------------------|--------------|
| BtcMinerProxy      | **High**    | **Yes**     | **0%**      | **Yes**        | **Yes**           | **Yes**      |
| XMRig-Proxy    | **High**    | No          | 2%          | **Yes**        | **Yes**           | No           |
| Snipa22/xmr-node-proxy | Moderate | No     | 1%          | No             | **Yes**           | No           |

## License
BtcMinerProxy is licensed under [GPLv3](LICENSE).

## Contact