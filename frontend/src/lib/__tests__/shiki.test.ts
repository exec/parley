import { describe, it, expect } from 'vitest';
import { languageFromFilename, isCodeFile } from '../shiki';

describe('languageFromFilename', () => {
  const cases: [string, string][] = [
    ['main.py', 'python'],
    ['server.go', 'go'],
    ['lib.rs', 'rust'],
    ['app.js', 'javascript'],
    ['component.jsx', 'jsx'],
    ['utils.ts', 'typescript'],
    ['page.tsx', 'tsx'],
    ['run.sh', 'bash'],
    ['run.bash', 'bash'],
    ['run.zsh', 'bash'],
    ['script.ps1', 'powershell'],
    ['mod.lua', 'lua'],
    ['main.c', 'c'],
    ['header.h', 'c'],
    ['main.cpp', 'cpp'],
    ['header.hpp', 'cpp'],
    ['config.yaml', 'yaml'],
    ['config.yml', 'yaml'],
    ['data.json', 'json'],
    ['config.toml', 'toml'],
    ['app.rb', 'ruby'],
    ['Main.java', 'java'],
    ['boot.asm', 'asm'],
    ['boot.s', 'asm'],
    ['index.html', 'html'],
    ['style.css', 'css'],
    ['style.scss', 'scss'],
    ['query.sql', 'sql'],
    ['README.md', 'markdown'],
    ['Dockerfile.dockerfile', 'dockerfile'],
    ['infra.tf', 'hcl'],
    ['layout.xml', 'xml'],
  ];

  it.each(cases)('maps %s to %s', (filename, expected) => {
    expect(languageFromFilename(filename)).toBe(expected);
  });

  it('returns empty string for unknown extensions', () => {
    expect(languageFromFilename('file.unknown')).toBe('');
    expect(languageFromFilename('file.txt')).toBe('');
  });

  it('handles uppercase extensions', () => {
    expect(languageFromFilename('file.PY')).toBe('python');
    expect(languageFromFilename('file.TS')).toBe('typescript');
  });
});

describe('isCodeFile', () => {
  it('returns true for known code files', () => {
    expect(isCodeFile('main.py')).toBe(true);
    expect(isCodeFile('app.tsx')).toBe(true);
    expect(isCodeFile('Makefile.go')).toBe(true);
  });

  it('returns false for unknown extensions', () => {
    expect(isCodeFile('readme.txt')).toBe(false);
    expect(isCodeFile('photo.png')).toBe(false);
    expect(isCodeFile('file.unknown')).toBe(false);
  });
});
