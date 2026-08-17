import http from 'node:http';

http.createServer((req, res) => {
  if (req.url === '/api/data') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ hello: 'world' }));
    return;
  }
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(`<!DOCTYPE html>
<html>
<head><title>目标站点</title></head>
<body>
  <h1>原始内容</h1>
  <p>这里没有任何注入。</p>
</body>
</html>
`);
}).listen(8000, () => console.log('目标站点 :8000'));
