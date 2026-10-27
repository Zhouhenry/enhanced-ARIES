"""The pinned native FS/Shell models over an attempt-private transport."""
import base64

PROTOCOL = 1
METHODS = {
    'fs': {
        'read_file': 'ReadFileResult',
        'read_file_stream': 'ReadFileStreamResult',
        'write_file': 'WriteFileResult',
        'list_files': 'ListFilesResult',
        'list_directories': 'ListDirsResult',
        'search_files': 'SearchFilesResult',
    },
    'shell': {
        'execute_cmd': 'ExecuteCmdResult',
        'execute_cmd_stream': 'ExecuteCmdStreamResult',
        'execute_cmd_background': 'ExecuteCmdBackgroundResult',
    },
}


def encode_fields(value, fields):
    value = dict(value)
    binary = []
    for key in fields:
        if isinstance(value.get(key), bytes):
            value[key] = base64.b64encode(value[key]).decode('ascii')
            binary.append(key)
    return value, binary


def decode_fields(value, binary, fields):
    value = dict(value)
    if not isinstance(binary, list) or len(set(binary)) != len(binary):
        raise ValueError('invalid binary field declaration')
    for key in binary:
        if key not in fields or not isinstance(value.get(key), str):
            raise ValueError('invalid binary field')
        value[key] = base64.b64decode(value[key], validate=True)
    return value


def result_frame(result, invocation_id):
    value = result.model_dump(mode='python')
    binary = []
    if isinstance(value.get('data'), dict):
        value['data'], binary = encode_fields(value['data'], ('content', 'chunk_content'))
    return {'event': 'result', 'invocation_id': invocation_id,
            'result': value, 'result_bytes': binary}


def native_result(kind, method, frame):
    from openjiuwen.core.sys_operation import result as models
    value = dict(frame['result'])
    if isinstance(value.get('data'), dict):
        value['data'] = decode_fields(value['data'], frame.get('result_bytes', []),
                                      ('content', 'chunk_content'))
    elif frame.get('result_bytes'):
        raise ValueError('binary declaration without result data')
    return getattr(models, METHODS[kind][method]).model_validate(value)
