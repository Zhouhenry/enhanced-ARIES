"""Prepared AgentSphere request bodies only; no network calls or cloud creation."""
import shlex


def create_body(name, gateway_id):
    # Same resource request as the existing successful codingcase B template.
    return {'name': name, 'cpuCount': 2, 'memoryMB': 4096,
            'gatewayID': gateway_id, 'arch': 'x86_64'}


def build_body(image_reference):
    if '@sha256:' not in image_reference:
        raise ValueError('use the published immutable image reference')
    health = "import urllib.request; r=urllib.request.urlopen('http://127.0.0.1:49983/health',timeout=3); assert r.status==204"
    return {'fromImage': image_reference, 'steps': [],
            'readyCmd': 'python3 -I -c ' + shlex.quote(health),
            'networkConfiguration': {'networkMode': 'public'},
            'invoke': {'protocol': 'HTTP', 'port': 49983}, 'storageConfig': {}}
