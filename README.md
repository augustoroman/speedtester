A simple speedtesting app.

Usage:

`bash
   go get github.com/augustoroman/speedtester
   # On one machine, assuming it's IP is 192.168.1.3
   speedtester serve
   # On the other machine:
   speedtester upload 129.168.1.3:5555
   speedtester download 129.168.1.3:5555
`
