"""Prepared E2B SDK definition only; importing never creates cloud resources."""
RESOURCES = {'cpu_count': 2, 'memory_mb': 4096}


def definition(template_class, image_reference):
    # Supply the image's actual published digest after local acceptance.
    if '@sha256:' not in image_reference:
        raise ValueError('use the published immutable image reference')
    return (template_class().from_image(image_reference)
            .set_user('root').set_workdir('/workspace/aries-case')
            .set_start_cmd('exec /bin/sh /opt/aries-template/start.sh',
                           'test -f /opt/aries-template/ready.json'))
