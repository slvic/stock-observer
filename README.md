# STOCK OBSERVER

it fetches info from binance p2p and Bestchange

simple as

### technologies:
- grafana
- prometheus
- docker

### fetching processes:
- Binance P2P
- Bestchange

### how to use:
- start the app
    ```bash
    $ docker-compose up
    ```
- get data
    - go-to `http://localhost3000`
    - authorize with
      - login: `admin`
      - password: `admin`
    - create desired dashboards