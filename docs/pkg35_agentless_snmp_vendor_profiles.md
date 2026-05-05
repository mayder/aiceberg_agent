# PKG-35 - Agentless SNMP com perfil proprietario

Este documento cobre a operacao do agente Go quando a web envia um catalogo proprietario de OIDs SNMP no job Agentless.

## Contrato do job

O agente aceita, dentro de `config`, os campos:

- `vendor_oid_profile`: metadados do perfil aplicado ou candidato, incluindo `profile_key`, `profile_version`, `vendor`, `family`, `model`, `source_mib`, `matched_by`, `match_source`, `applied` e `last_validated_at`;
- `custom_get_oids`: lista de OIDs escalares, no formato legado de string ou no formato nomeado com `name`, `oid`, `label`, `metric`, `source_mib`, `unit` e `canonical_key`;
- `custom_walk_oids`: lista de OIDs de walk com os mesmos metadados nomeados;
- `fallback_groups`: grupos genericos que continuam habilitados quando o perfil proprietario existe.

O agente coleta exatamente os OIDs recebidos. Ele nao interpreta MIB local, nao carrega arquivo `.mib` do host e nao inventa OID proprietario quando a web nao envia perfil.

## Payload de retorno

Quando um perfil proprietario participa da coleta, a observacao enviada para `/v1/hub-agentless/observations` inclui:

- `vendor_oid_profile`;
- `vendor_profile_applied`;
- `profile_key`;
- `profile_version`;
- `oids_success`;
- `oids_failed`;
- `fallback_used`;
- `cpu_percent` e `memory_percent`, quando algum OID vier com `canonical_key` correspondente;
- `cpu_memory.source_profile` e `cpu_memory.vendor_samples`;
- dados brutos preservados em `custom`, `oids`, `scalars` e `tables`.

Falha em um OID proprietario deve aparecer em `oids_failed` e nao deve derrubar a coleta inteira quando os demais OIDs responderem.

## Validacao com SNMP externo

Antes de culpar o agente, valide conectividade e permissao SNMP pelo mesmo ponto de rede do coletor.

```bash
HOST="192.0.2.10"
COMMUNITY="public"

snmpget -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.2.1.1.1.0
snmpget -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.2.1.1.2.0
```

OmniSwitch/AOS:

```bash
snmpget -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.15.0
snmpget -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.16.0
snmpwalk -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1
```

OAW AP:

```bash
snmpget -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.8.0
snmpget -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.7.0
snmpwalk -v2c -c "$COMMUNITY" "$HOST" 1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1
```

## Roteiro E2E

1. Na web, deixe o check SNMP sem perfil proprietario aplicado.
2. Execute a primeira coleta para capturar `sysObjectID` e `sysDescr`.
3. Confirme no proximo job recebido pelo agente que `config.vendor_oid_profile` e os OIDs proprietarios existem somente quando a web encontrou match ALE/OAW.
4. Confirme no log/payload que o agente executou os OIDs de `custom_get_oids` e `custom_walk_oids` exatamente como recebidos.
5. Confirme que a observacao retornou `vendor_profile_applied=true`, `oids_success`, `oids_failed`, `fallback_used`, `cpu_percent` e `memory_percent` quando os OIDs responderam.
6. Confirme na web que o perfil foi persistido como aplicado e que a segunda coleta recebeu `applied=true` e `proprietary_once=false`.
7. Simule uma falha parcial em um OID. O payload deve manter os OIDs bons em `oids_success`, registrar o OID ruim em `oids_failed` e preservar status util da coleta.

## Rollback

Rollback sem trocar binario:

1. Resetar o perfil proprietario no detalhe Agentless da web.
2. Confirmar que o proximo job chega sem `vendor_oid_profile` quando nao houver nova evidencia ALE/OAW.
3. Se a web continuar identificando o mesmo equipamento e reaplicar o perfil, desabilitar o perfil no `config_json` do check com `vendor_oid_profile.enabled=false`.
4. Confirmar que o agente voltou ao plano generico atual e que `fallback_used`/estado da tela indicam coleta generica.

Rollback de versao segue o gate de `GOVERNANCA.md` do projeto web: publicar artefato anterior do agente antes de acionar update remoto.
