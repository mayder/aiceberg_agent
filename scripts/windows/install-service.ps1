param(
  [string]\ = 'C:\Program Files\AIceberg\agent\agent.exe',
  [string]\ = 'C:\ProgramData\AIceberg\config.yml'
)
sc.exe create AIcebergAgent binPath= '\"' + \ + '\" -config \"' + \ + '\"' start= auto
sc.exe start AIcebergAgent
