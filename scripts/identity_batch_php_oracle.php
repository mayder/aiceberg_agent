<?php
// Synthetic signature interoperability only: no application, database or network writes.
$root = getenv('AICEBERG_WEB_ROOT');
if (!$root || !is_file($root . '/vendor/autoload.php')) exit(2);
require $root . '/vendor/autoload.php';
require $root . '/vendor/yiisoft/yii2/Yii.php';
require $root . '/common/config/bootstrap.php';
final class IdentityBatchFixtureAgent extends common\models\Agente
{
    public function attributes(): array { return ['id', 'cliente_id', 'token', 'status', 'modo']; }
}
$agent = new IdentityBatchFixtureAgent();
$agent->id = 42;
$agent->cliente_id = 7;
$agent->token = 'synthetic-secret';
$claim = json_decode(stream_get_contents(STDIN, 65536), true, 32, JSON_THROW_ON_ERROR);
$service = new common\services\agent\AgentIdentityClaimService();
$valid = $service->validateClaim($agent, $claim);
$claim['agente_id'] = 43;
$changed = $service->validateClaim($agent, $claim);
echo json_encode(['trusted' => $valid['trusted'], 'changed_subject_rejected' => !$changed['trusted']], JSON_THROW_ON_ERROR);
