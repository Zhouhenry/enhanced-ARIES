"""Offline UID 60000 fixture for native skill discovery and PPT generation/rendering."""
import argparse
import asyncio
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path
import subprocess
import sys
import zipfile


BUILTINS = Path('/opt/jiuwenswarm/jiuwenswarm/resources/agent/workspace/skills')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    assert os.getuid() == 60000
    out = args.output.resolve()
    out.mkdir(parents=True, exist_ok=True)
    home = out / 'home'
    home.mkdir()
    os.environ.update(HOME=str(home), JIUWENSWARM_DATA_DIR=str(home / 'data'),
                      JIUWENSWARM_CONFIG_DIR=str(home / 'data/config'),
                      JIUWENSWARM_SHARED_SKILLS_DIRS=str(BUILTINS))
    os.chdir(out)
    receipt = {'status': 'FAIL', 'uid': os.getuid(), 'model_calls': 0,
               'image_id': os.environ['ARIES_FIXTURE_IMAGE_ID'],
               'recipe_manifest_sha256': os.environ['ARIES_FIXTURE_INPUT_SHA256'],
               'scope': 'generic environment fixture; not a benchmark task or scored Agent response'}
    def run(label, argv):
        p = subprocess.run(argv, capture_output=True, text=True, timeout=180)
        (out / (label + '.stdout')).write_text(p.stdout)
        (out / (label + '.stderr')).write_text(p.stderr)
        receipt.setdefault('commands', []).append({'label': label, 'argv': argv, 'exit_code': p.returncode})
        if p.returncode:
            raise RuntimeError(label + ' failed; inspect retained output')
        return p.stdout
    try:
        from jiuwenswarm.common.utils import prepare_workspace, get_agent_skills_dir, get_agent_workspace_dir
        prepare_workspace(overwrite=False)
        from jiuwenswarm.common.config import get_config
        from jiuwenswarm.server.runtime.agent_adapter.interface_flash import JiuwenSwarmFlashAdapter
        from jiuwenswarm.server.runtime.skill.skill_manager import SkillManager
        from openjiuwen.core.foundation.llm.model import init_model
        from openjiuwen.core.single_agent.schema.agent_card import AgentCard
        from openjiuwen.core.sys_operation import LocalWorkConfig, OperationMode, SysOperationCard
        from openjiuwen.core.runner.runner import Runner
        from openjiuwen.harness.factory import create_deep_agent
        async def discover():
            adapter = JiuwenSwarmFlashAdapter()
            adapter._skill_manager = SkillManager()
            rail = adapter._build_skill_rail(get_config().get('react', {}), include_tools=False)
            assert rail is not None
            await rail.reload_skills()
            catalog = {s.name: str(s.directory) for s in rail.skills_meta}
            expected = {p.name for p in BUILTINS.iterdir() if (p / 'SKILL.md').is_file()}
            assert set(catalog) == expected and len(expected) == 20
            assert all(Path(path).parent == BUILTINS for path in catalog.values())
            assert not (get_agent_skills_dir() / 'ppt-creation').exists()
            card = SysOperationCard(id='presentation_fixture', mode=OperationMode.LOCAL,
                                   work_config=LocalWorkConfig(work_dir=str(out)))
            Runner.resource_mgr.add_sys_operation(card)
            model = init_model(provider='OpenAI', model_name='never-called', api_key='not-a-key',
                               api_base='http://127.0.0.1:1/v1')
            agent = create_deep_agent(model=model, card=AgentCard(name='presentation_fixture'),
                                     workspace=get_agent_workspace_dir(),
                                     sys_operation=Runner.resource_mgr.get_sys_operation(card.id),
                                     enable_task_loop=False, enable_security_rail=False,
                                     enable_llm_retry_rail=False)
            await agent.register_rail(rail)
            tool_card = next(c for c in agent.ability_manager.list() if c.name == 'skill_tool')
            result = await Runner.resource_mgr.get_tool(tool_card.id).invoke({'skill_name': 'ppt-creation'})
            assert result.success and 'PPT Creation' in result.data['content']
            receipt['skills'] = {'catalog': catalog, 'count': len(catalog),
                                 'ppt_skill_tool_read': True, 'ppt_private_copy': False,
                                 'skill_content_sha256': hashlib.sha256(result.data['content'].encode()).hexdigest()}
            await agent.unregister_rail(rail)
        asyncio.run(discover())
        script = out / 'fixture.js'
        script.write_text("""const PptxGenJS = require('pptxgenjs');
const p = new PptxGenJS(); p.defineLayout({name:'NATIVE_CONTENT',width:10,height:5.625}); p.layout='NATIVE_CONTENT';
p.author='ARIES environment fixture'; p.subject='Unscored environment validation';
const s=p.addSlide(); s.background={color:'FFFFFF'};
s.addText('环境验证 / Environment check',{x:0.6,y:0.5,w:8.8,h:0.7,fontFace:'Noto Sans CJK SC',fontSize:26,color:'4472C4'});
s.addText('Native PptxGenJS content, template merge, and offline rendering.',{x:0.6,y:1.6,w:8.8,h:1.0,fontFace:'Liberation Sans',fontSize:20,color:'222222'});
s.addText('这是一份通用环境夹具，不是用户任务答案。',{x:0.6,y:3.0,w:8.8,h:0.8,fontFace:'Noto Sans CJK SC',fontSize:20,color:'222222'});
p.writeFile({fileName:'content.pptx'});
""")
        skill = BUILTINS / 'ppt-creation'
        run('node-generate', ['node', str(script)])
        run('native-finalize', ['python3', str(skill / 'scripts/finalize_deck.py'),
                               'content.pptx', 'fixture.pptx', '--cover-title', 'Environment validation',
                               '--cover-meta', 'ARIES|Fixture|2026-10-27'])
        run('native-thumbnail', ['python3', str(skill / 'scripts/opc/thumbnail.py'),
                                'fixture.pptx', 'contact', '--cols', '3'])
        run('libreoffice-render', ['soffice', '--headless', '--convert-to', 'pdf',
                                   'fixture.pptx', '--outdir', str(out)])
        run('poppler-render', ['pdftoppm', '-f', '2', '-singlefile', '-png', '-r', '100',
                              'fixture.pdf', 'content-page'])
        from pptx import Presentation
        import fitz
        from PIL import Image
        deck = Presentation(out / 'fixture.pptx')
        assert len(deck.slides) == 3
        with fitz.open(out / 'fixture.pdf') as pdf:
            assert len(pdf) == 3
            text = ''.join(page.get_text() for page in pdf)
            assert 'Environment check' in text and '环境验证' in text
            receipt['pdf_pages'] = len(pdf)
        with Image.open(out / 'content-page.png') as img:
            assert img.width > 500 and img.height > 200
            receipt['render_size'] = [img.width, img.height]
        receipt['versions'] = {'node': run('node-version', ['node', '--version']).strip(),
                               'npm': run('npm-version', ['npm', '--version']).strip(),
                               'libreoffice': run('lo-version', ['soffice', '--version']).strip(),
                               'python': sys.executable,
                               'pymupdf': importlib.metadata.version('PyMuPDF')}
        receipt['artifacts'] = {p.name: sha(p) for p in out.iterdir()
                                if p.is_file() and p.suffix in ('.pptx', '.pdf', '.png', '.jpg')}
        assert receipt['artifacts'].get('content-page.png') and any(k.endswith('.jpg') for k in receipt['artifacts'])
        receipt['status'] = 'PASS_NATIVE_SKILLS20_AND_PPT_GENERATION_RENDERING_FIXTURE'
    finally:
        (out / 'evidence.json').write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(receipt, ensure_ascii=False))


if __name__ == '__main__':
    main()
