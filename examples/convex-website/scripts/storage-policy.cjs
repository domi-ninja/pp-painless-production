// The backend can read and write objects in its declared buckets, without admin access.
function storagePolicy(buckets) {
  if (!buckets.length || buckets.some(bucket => !/^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/.test(bucket))) {
    throw new Error('Invalid storage bucket name');
  }
  const resources = [...new Set(buckets)].map(bucket => `arn:aws:s3:::${bucket}`);
  return { Version: '2012-10-17', Statement: [
    { Effect: 'Allow', Action: ['s3:GetBucketLocation', 's3:ListBucket', 's3:ListBucketMultipartUploads'], Resource: resources },
    { Effect: 'Allow', Action: ['s3:GetObject', 's3:PutObject', 's3:DeleteObject', 's3:AbortMultipartUpload', 's3:ListMultipartUploadParts'], Resource: resources.map(resource => `${resource}/*`) },
  ] };
}
module.exports = { storagePolicy };
if (require.main === module) {
  try { process.stdout.write(JSON.stringify(storagePolicy(process.argv.slice(2)))); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
