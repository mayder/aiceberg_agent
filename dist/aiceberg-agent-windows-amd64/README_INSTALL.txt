AIceberg Agent - pacote standalone (Windows)

Conteúdo:
- agent.exe
- install-service.ps1 (cria o serviço)
- install.ps1 (instala tudo: copia binário, grava token/env, cria serviço)
- LEIA-ME: defina AGENT_TOKEN (variável de ambiente do sistema) e paths opcionais; ou crie C:\ProgramData\AIceberg\agent.token e sete AGENT_TOKEN_PATH.

Passos resumidos (PowerShell como Admin):
1) (Recomendado) Rodar instalador automático:
   powershell -ExecutionPolicy Bypass -File .\install.ps1 -Token SEU_TOKEN_AQUI
   (isso copia o binário, grava token/env e cria o serviço)
2) Ou manual:
   - Copie agent.exe para "C:\Program Files\AIceberg\agent"
   - Crie dados: mkdir C:\ProgramData\AIceberg
   - setx /M AGENT_TOKEN_PATH C:\ProgramData\AIceberg\agent.token
   - echo -n SEU_TOKEN_AQUI > C:\ProgramData\AIceberg\agent.token
   - Execute: .\install-service.ps1 -BinPath "C:\Program Files\AIceberg\agent\agent.exe"
3) Verifique: sc.exe query AIcebergAgent ou Event Viewer (Application).

  API de produção já é padrão: https://api.aiceberg.com.br (o agente adiciona `/v1/...`).
