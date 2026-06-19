# PKG-72 - Replay offline 24h

## Ambiente

- Data UTC: 2026-06-19T19:30:00Z
- Responsavel: Codex
- Cliente/lab: lab-controlado-aiceberg
- Host/agente/HUB/relay: BoltStore local em tempdir com replay horario controlado
- Versao agente: 0.8.10
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

- Inicio/fim da janela offline: 2026-06-18T00:00:00Z ate 2026-06-18T23:00:00Z.
- Quantidade de envelopes acumulados: 24 envelopes horarios offline-24h-00 ate offline-24h-23.
- Replay apos retorno da API: dois Peeks antes do ACK retornam os mesmos IDs na mesma ordem; Delete remove somente apos ACK.
- Duplicatas observadas: ACK com IDs duplicados e desconhecidos tratado de forma idempotente; fila final zerada.
- Topologia relay -> HUB -> AIceberg preservada quando aplicavel: TestBuildOfflineFirstEvidenceRelayKeepsHubOnlyTopology exige relay_to_hub_only=true e direct_api_from_relay=false.

## Metricas

- offline_replay_success: 24/24 envelopes preservados ate ACK.
- duplicate_rate: 0 apos ACK idempotente; replays antes do ACK sao duraveis por contrato.
- support_export_integrity: local_export.signed=true com signature_algorithm=sha256 e assinatura de 64 caracteres no snapshot contextual.

## Resultado

- Status: pass
- Evidencia bruta anexada: internal/data/local/outbox/bolt_store_test.go TestBoltStoreSimulates24hOfflineReplayWithoutDuplicatesAfterAck e internal/bootstrap/runtime_snapshot_test.go TestBuildOfflineFirstEvidenceRelayKeepsHubOnlyTopology.
- Observacoes: simulacao controlada de 24 horas; cobre contrato local de durabilidade, idempotencia e topologia Relay -> HUB -> AIceberg.
- Rollback validado: desligar contextual_evidence nao altera outbox nem transporte.
- Revisor: Codex
- Aprovacao fechamento: yes
