AIceberg Agent - pacote standalone (Windows)

Conteúdo:
- agent.exe
- install-service.ps1 (cria o serviço)
- LEIA-ME: defina AGENT_TOKEN (variável de ambiente do sistema) e paths opcionais; ou crie C:\ProgramData\AIceberg\agent.token e sete AGENT_TOKEN_PATH.

Passos resumidos (PowerShell como Admin):
1) Copie agent.exe para "C:\Program Files\AIceberg\agent".
2) Crie dados: mkdir C:\ProgramData\AIceberg
3) Defina o token (recomendado com path explícito):
   - setx /M AGENT_TOKEN_PATH C:\ProgramData\AIceberg\agent.token
   - echo -n SEU_TOKEN_AQUI > C:\ProgramData\AIceberg\agent.token
   (ou setx /M AGENT_TOKEN SEU_TOKEN_AQUI)
4) Execute: .\install-service.ps1 -BinPath "C:\Program Files\AIceberg\agent\agent.exe"
5) Verifique: sc.exe query AIcebergAgent ou Event Viewer (Application).

  API de produção já é padrão: https://api.aiceberg.com.br (o agente adiciona `/v1/...`).
